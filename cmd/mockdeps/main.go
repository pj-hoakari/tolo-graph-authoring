package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	connectrpc "connectrpc.com/connect"
	"github.com/pj-hoakari/internal-jwt-handling/jwtgen"

	tenantv1 "github.com/pj-hoakari/tolo-tenant-management/gen/tolo/tenant/v1"
	"github.com/pj-hoakari/tolo-tenant-management/gen/tolo/tenant/v1/tenantv1connect"

	connectinfra "github.com/pj-hoakari/tolo-graph-authoring/internal/infra/connect"
)

const (
	tenantPublicID = "0123456789abcdef"
	eventPublicID  = "fedcba9876543210"
)

func main() {
	generator, err := jwtgen.NewGenerator("")
	if err != nil {
		slog.Error("create signing key failed", "error", err)
		os.Exit(1)
	}

	mux, err := newMux(generator)
	if err != nil {
		slog.Error("build handler failed", "error", err)
		os.Exit(1)
	}

	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	slog.Info("mockdeps listening", "addr", server.Addr)

	if err := server.ListenAndServe(); err != nil {
		slog.Error("mockdeps failed", "error", err)
		os.Exit(1)
	}
}

func newMux(generator *jwtgen.Generator) (*http.ServeMux, error) {
	jwks, err := generator.JWKS()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle(tenantv1connect.NewTenantServiceHandler(tenantService{
		UnimplementedTenantServiceHandler: tenantv1connect.UnimplementedTenantServiceHandler{},
	}))
	mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jwks)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		issueToken(w, r, generator)
	})

	return mux, nil
}

type tokenRequest struct {
	Audience string `json:"audience"`
	TokenUse string `json:"token_use"`
	TenantID string `json:"tenant_id"`
	EventID  string `json:"event_id"`
	Scope    string `json:"scope"`
	TTL      string `json:"ttl"`
}

func issueToken(w http.ResponseWriter, r *http.Request, generator *jwtgen.Generator) {
	req := tokenRequest{
		Audience: connectinfra.DefaultInternalJWTAudience,
		TokenUse: "",
		TenantID: "",
		EventID:  "",
		Scope:    "",
		TTL:      "10m",
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	ttl, err := time.ParseDuration(req.TTL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	output, err := generator.Generate(jwtgen.Config{
		Issuer:         connectinfra.DefaultInternalJWTIssuer,
		Audience:       req.Audience,
		TokenUse:       req.TokenUse,
		TenantPublicID: req.TenantID,
		EventPublicID:  req.EventID,
		Scope:          req.Scope,
		OriginSub:      "",
		Txn:            "",
		Subject:        "",
		KeyID:          "",
		TTL:            ttl,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	writeJSON(w, output)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response failed", "error", err)
	}
}

type tenantService struct {
	tenantv1connect.UnimplementedTenantServiceHandler
}

func (tenantService) GetEvent(
	_ context.Context, req *connectrpc.Request[tenantv1.GetEventRequest],
) (*connectrpc.Response[tenantv1.GetEventResponse], error) {
	if req.Msg.GetEventId() != eventPublicID {
		return nil, connectrpc.NewError(connectrpc.CodeNotFound, nil)
	}

	return connectrpc.NewResponse(&tenantv1.GetEventResponse{Event: &tenantv1.Event{
		EventId:  eventPublicID,
		TenantId: tenantPublicID,
		Name:     "Development Event",
		Type:     tenantv1.EventType_EVENT_TYPE_SHORT_TERM,
		Status:   tenantv1.EventStatus_EVENT_STATUS_OPEN,
	}}), nil
}
