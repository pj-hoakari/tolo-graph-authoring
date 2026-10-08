// Package connect provides the Connect HTTP transport for this service.
package connect

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	connectrpc "connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	"github.com/pj-hoakari/internal-jwt-handling/interceptor"
	"github.com/pj-hoakari/internal-jwt-handling/jwks"
	"github.com/pj-hoakari/internal-jwt-handling/verifier"

	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
)

// Defaults for verifying internal JWTs. The issuer is the Service Gateway's
// issuer identifier and must match the value the gateway signs with; the
// audience is this service's logical identifier.
const (
	DefaultInternalJWKSURL     = "http://gateway:8080/.well-known/jwks.json"
	DefaultInternalJWTIssuer   = "service-gateway"
	DefaultInternalJWTAudience = "tolo-graph-authoring"
)

// JWTSettings locates the Service Gateway's JWKS and names the issuer and
// audience every internal JWT must carry.
type JWTSettings struct {
	JWKSURL  string
	Issuer   string
	Audience string
}

// DefaultJWTSettings returns the settings for the Docker Compose setup;
// cmd/server overrides them from the environment.
func DefaultJWTSettings() JWTSettings {
	return JWTSettings{
		JWKSURL:  DefaultInternalJWKSURL,
		Issuer:   DefaultInternalJWTIssuer,
		Audience: DefaultInternalJWTAudience,
	}
}

// RoutesWithJWTSettings builds the service routes that verify internal
// JWTs against the JWKS the settings locate.
func RoutesWithJWTSettings(
	graphService application.GraphUseCases,
	settings JWTSettings,
) (func(mux *http.ServeMux), error) {
	cache, err := jwks.New(jwks.Config{
		URL:             settings.JWKSURL,
		HTTPClient:      nil,
		CacheTTL:        0,
		RefreshCooldown: 0,
		FailureCooldown: 0,
		FetchTimeout:    0,
		RetryBackoff:    nil,
		MaxDocumentSize: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("create JWKS cache: %w", err)
	}

	tokenVerifier, err := verifier.New(settings.Issuer, settings.Audience, cache)
	if err != nil {
		return nil, fmt.Errorf("create internal JWT verifier: %w", err)
	}

	return RoutesWithVerifier(graphService, tokenVerifier)
}

// RoutesWithVerifier builds the service routes around a verifier of the
// internal JWT. The service is guarded by an interceptor built from its
// generated policy table, so the credential rules stay declared in the proto.
func RoutesWithVerifier(
	graphService application.GraphUseCases,
	tokenVerifier interceptor.TokenVerifier,
) (func(mux *http.ServeMux), error) {
	// The caller sits behind the Service Gateway, so an incoming trace context is
	// trusted and continued instead of being demoted to a span link.
	tracing, err := otelconnect.NewInterceptor(otelconnect.WithTrustRemote())
	if err != nil {
		return nil, fmt.Errorf("create tracing interceptor: %w", err)
	}

	graphAuth, err := interceptor.New(
		tokenVerifier,
		graphv1connect.GraphAuthoringServicePolicies,
		interceptor.WithErrorReporter(reportAuthRejection),
	)
	if err != nil {
		return nil, fmt.Errorf("create GraphAuthoringService authentication interceptor: %w", err)
	}

	// Tracing runs before authentication, so a rejected call is still recorded
	// on the trace it belongs to.
	graphPath, graphHandler := graphv1connect.NewGraphAuthoringServiceHandler(
		NewGraphService(graphService),
		connectrpc.WithInterceptors(tracing, graphAuth, forwardAuthorization()),
	)

	supplyAuth, err := interceptor.New(
		tokenVerifier,
		graphv1connect.GraphSupplyServicePolicies,
		interceptor.WithErrorReporter(reportAuthRejection),
	)
	if err != nil {
		return nil, fmt.Errorf("create GraphSupplyService authentication interceptor: %w", err)
	}

	supplyPath, supplyHandler := graphv1connect.NewGraphSupplyServiceHandler(
		NewGraphSupplyService(graphService),
		connectrpc.WithInterceptors(tracing, supplyAuth),
	)

	return func(mux *http.ServeMux) {
		mux.Handle(graphPath, graphHandler)
		mux.Handle(supplyPath, supplyHandler)
	}, nil
}

// reportAuthRejection logs why a call was refused. The client only ever learns
// the Connect code, so the cause is kept server-side, on the trace of the
// request context.
func reportAuthRejection(ctx context.Context, procedure string, err error) {
	slog.WarnContext(ctx, "internal JWT rejected", "procedure", procedure, "error", err)
}
