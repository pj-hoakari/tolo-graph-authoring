package connect

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/internal-jwt-handling/jwks"
	"github.com/pj-hoakari/internal-jwt-handling/jwtgen"
	"github.com/pj-hoakari/internal-jwt-handling/verifier"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

// newTestJWKSURL serves keys from an httptest endpoint, mirroring the Service
// Gateway publishing its signing keys, and returns the URL to fetch them from.
func newTestJWKSURL(t *testing.T, keys internaljwt.JWKS) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(keys); err != nil {
			t.Errorf("encode JWKS: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return server.URL
}

// newTestVerifier builds a verifier backed by an httptest JWKS endpoint serving
// keys.
func newTestVerifier(t *testing.T, keys internaljwt.JWKS) *verifier.Verifier {
	t.Helper()

	// The cooldowns are collapsed so that a refresh on an unknown kid is never
	// held off within a test run.
	cache, err := jwks.New(jwks.Config{
		URL:             newTestJWKSURL(t, keys),
		RefreshCooldown: time.Nanosecond,
		FailureCooldown: time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("create JWKS cache: %v", err)
	}

	tokenVerifier, err := verifier.New(DefaultInternalJWTIssuer, DefaultInternalJWTAudience, cache)
	if err != nil {
		t.Fatalf("create internal JWT verifier: %v", err)
	}

	return tokenVerifier
}

// newTestHandler builds a handler serving the production service routes wired
// to a verifier trusting keys, serving graphService.
func newTestHandler(t *testing.T, keys internaljwt.JWKS, graphService application.GraphUseCases) http.Handler {
	t.Helper()

	routes, err := RoutesWithVerifier(graphService, newTestVerifier(t, keys))
	if err != nil {
		t.Fatalf("RoutesWithVerifier() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	return mux
}

func newTestHandlerForJWKSURL(t *testing.T, jwksURL string) http.Handler {
	t.Helper()

	cache, err := jwks.New(jwks.Config{
		URL:             jwksURL,
		RefreshCooldown: time.Nanosecond,
		FailureCooldown: time.Nanosecond,
		RetryBackoff:    []time.Duration{},
	})
	if err != nil {
		t.Fatalf("create JWKS cache: %v", err)
	}

	tokenVerifier, err := verifier.New(DefaultInternalJWTIssuer, DefaultInternalJWTAudience, cache)
	if err != nil {
		t.Fatalf("create internal JWT verifier: %v", err)
	}

	routes, err := RoutesWithVerifier(newTestGraphService(), tokenVerifier)
	if err != nil {
		t.Fatalf("RoutesWithVerifier() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	return mux
}

// mintInternalJWT issues an internal JWT for the issuer and audience this
// service verifies against.
func mintInternalJWT(t *testing.T, tokenUse, scope, tenantPublicID string) (string, internaljwt.JWKS) {
	t.Helper()

	return mintJWT(t, jwtgen.Config{TokenUse: tokenUse, TenantPublicID: tenantPublicID, Scope: scope})
}

func mintEventAccessJWT(t *testing.T, tenantPublicID, eventPublicID string) (string, internaljwt.JWKS) {
	t.Helper()

	return mintJWT(t, jwtgen.Config{
		TokenUse:       internaljwt.TokenUseEventAccess,
		TenantPublicID: tenantPublicID,
		EventPublicID:  eventPublicID,
		Scope:          "events.manage",
	})
}

func mintJWT(t *testing.T, config jwtgen.Config) (string, internaljwt.JWKS) {
	t.Helper()

	if config.Issuer == "" {
		config.Issuer = DefaultInternalJWTIssuer
	}

	if config.Audience == "" {
		config.Audience = DefaultInternalJWTAudience
	}

	config.KeyID = "test-key"
	config.TTL = time.Hour

	output, err := jwtgen.Generate(config)
	if err != nil {
		t.Fatalf("generate internal JWT: %v", err)
	}

	return "Bearer " + output.Token, output.JWKS
}

func saveGraphThrough(t *testing.T, handler http.Handler, authorization string) (*connectrpc.Response[graphv1.GraphMeta], error) {
	t.Helper()

	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := graphv1connect.NewGraphAuthoringServiceClient(httpServer.Client(), httpServer.URL)

	req := connectrpc.NewRequest(&graphv1.SaveGraphRequest{
		EventId: "fedcba9876543210",
		Document: &graphv1.GraphDocument{
			Nodes: []*graphv1.GraphNode{{NodeId: "n1", NodeType: graphv1.NodeType_NODE_TYPE_GOAL}},
		},
	})
	if authorization != "" {
		req.Header().Set("Authorization", authorization)
	}

	return client.SaveGraph(context.Background(), req)
}

func TestRoutesWithJWTSettings(t *testing.T) {
	t.Parallel()

	t.Run("verifies a token against the JWKS the settings locate", func(t *testing.T) {
		t.Parallel()

		authorization, keys := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")

		settings := DefaultJWTSettings()
		settings.JWKSURL = newTestJWKSURL(t, keys)

		routes, err := RoutesWithJWTSettings(newTestGraphService(), settings)
		if err != nil {
			t.Fatalf("RoutesWithJWTSettings() error = %v", err)
		}

		mux := http.NewServeMux()
		routes(mux)

		if _, err := saveGraphThrough(t, mux, authorization); err != nil {
			t.Fatalf("SaveGraph() error = %v", err)
		}
	})

	t.Run("rejects settings without a JWKS URL", func(t *testing.T) {
		t.Parallel()

		settings := DefaultJWTSettings()
		settings.JWKSURL = ""

		_, err := RoutesWithJWTSettings(newTestGraphService(), settings)
		if !errors.Is(err, jwks.ErrMissingURL) {
			t.Fatalf("RoutesWithJWTSettings() error = %v, want %v", err, jwks.ErrMissingURL)
		}
	})
}

func TestGraphAuthoringServiceAuthz(t *testing.T) {
	t.Parallel()

	authorization, keys := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")
	handler := newTestHandler(t, keys, newTestGraphService())

	t.Run("rejects missing bearer token", func(t *testing.T) {
		_, err := saveGraphThrough(t, handler, "")
		if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
			t.Fatalf("SaveGraph() error code = %v, want %v", got, want)
		}
	})

	t.Run("accepts internal JWT with required scope", func(t *testing.T) {
		if _, err := saveGraphThrough(t, handler, authorization); err != nil {
			t.Fatalf("SaveGraph() error = %v", err)
		}
	})
}

func TestGraphAuthoringServiceAuthzRejectsMissingScope(t *testing.T) {
	t.Parallel()

	authorization, keys := mintJWT(t, jwtgen.Config{
		TokenUse:       internaljwt.TokenUseEventAccess,
		TenantPublicID: "a1b2c3d4e5f60718",
		EventPublicID:  "fedcba9876543210",
		Scope:          "events.read",
	})

	_, err := saveGraphThrough(t, newTestHandler(t, keys, newTestGraphService()), authorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodePermissionDenied; got != want {
		t.Fatalf("SaveGraph() error code = %v, want %v", got, want)
	}
}

func TestGraphAuthoringServiceAuthzRejectsUnknownSigningKey(t *testing.T) {
	t.Parallel()

	// The handler trusts a JWKS publishing neither the key nor the kid that
	// signed the token below.
	_, trustedKeys := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")
	for i := range trustedKeys.Keys {
		trustedKeys.Keys[i].KeyID = "other-key"
	}

	foreignAuthorization, _ := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")

	_, err := saveGraphThrough(t, newTestHandler(t, trustedKeys, newTestGraphService()), foreignAuthorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("SaveGraph() error code = %v, want %v", got, want)
	}
}

func TestGraphAuthoringServiceAuthzUnavailableWhenJWKSUnreachable(t *testing.T) {
	t.Parallel()

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(jwksServer.Close)

	authorization, _ := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")

	_, err := saveGraphThrough(t, newTestHandlerForJWKSURL(t, jwksServer.URL), authorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnavailable; got != want {
		t.Fatalf("SaveGraph() error code = %v, want %v", got, want)
	}
}

func TestGraphAuthoringServiceAuthzRejectsServiceToken(t *testing.T) {
	t.Parallel()

	authorization, keys := mintInternalJWT(t, internaljwt.TokenUseService, "", "")

	_, err := saveGraphThrough(t, newTestHandler(t, keys, newTestGraphService()), authorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("SaveGraph() error code = %v, want %v", got, want)
	}
}

func TestGraphAuthoringServiceAuthzRejectsAudienceMismatch(t *testing.T) {
	t.Parallel()

	// The token names another service as its audience, so it is not a
	// credential this service may accept even though the key verifies.
	authorization, keys := mintJWT(t, jwtgen.Config{
		Audience:       "other-service",
		TokenUse:       internaljwt.TokenUseEventAccess,
		TenantPublicID: "a1b2c3d4e5f60718",
		EventPublicID:  "fedcba9876543210",
		Scope:          "events.manage",
	})

	_, err := saveGraphThrough(t, newTestHandler(t, keys, newTestGraphService()), authorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("SaveGraph() error code = %v, want %v", got, want)
	}
}

type savedGraphRepository struct {
	nopGraphRepository

	saved *domain.VenueGraph
}

func (r savedGraphRepository) Save(_ context.Context, graph domain.VenueGraph) error {
	*r.saved = graph

	return nil
}

func TestGraphAuthoringServiceInjectsTenantPublicID(t *testing.T) {
	t.Parallel()

	var saved domain.VenueGraph

	graphs := application.NewGraphService(savedGraphRepository{nopGraphRepository: nopGraphRepository{}, saved: &saved}, inlineTransactor{})
	authorization, keys := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")

	if _, err := saveGraphThrough(t, newTestHandler(t, keys, graphs), authorization); err != nil {
		t.Fatalf("SaveGraph() error = %v", err)
	}

	if got, want := saved.TenantPublicID(), "a1b2c3d4e5f60718"; got != want {
		t.Errorf("saved TenantPublicID() = %q, want %q", got, want)
	}
}
