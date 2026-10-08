package connect

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	connectrpc "connectrpc.com/connect"

	tenantv1 "github.com/pj-hoakari/tolo-tenant-management/gen/tolo/tenant/v1"
	"github.com/pj-hoakari/tolo-tenant-management/gen/tolo/tenant/v1/tenantv1connect"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
)

type fakeTenantService struct {
	tenantv1connect.UnimplementedTenantServiceHandler

	event          *tenantv1.Event
	err            error
	authorizations chan string
}

func (s *fakeTenantService) GetEvent(_ context.Context, req *connectrpc.Request[tenantv1.GetEventRequest]) (*connectrpc.Response[tenantv1.GetEventResponse], error) {
	s.authorizations <- req.Header().Get("Authorization")

	if s.err != nil {
		return nil, s.err
	}

	if s.event == nil || s.event.GetEventId() != req.Msg.GetEventId() {
		return nil, connectrpc.NewError(connectrpc.CodeNotFound, errors.New("event not found"))
	}

	return connectrpc.NewResponse(&tenantv1.GetEventResponse{Event: s.event}), nil
}

func newTenantServer(t *testing.T, service *fakeTenantService) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(tenantv1connect.NewTenantServiceHandler(service))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func tenantEvent(status tenantv1.EventStatus) *tenantv1.Event {
	return &tenantv1.Event{EventId: "fedcba9876543210", TenantId: "a1b2c3d4e5f60718", Status: status}
}

func TestTenantClientFindEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		event        *tenantv1.Event
		wantErr      error
		wantArchived bool
	}{
		{"open event", tenantEvent(tenantv1.EventStatus_EVENT_STATUS_OPEN), nil, false},
		{"archived event", tenantEvent(tenantv1.EventStatus_EVENT_STATUS_ARCHIVED), nil, true},
		{"unknown event", nil, application.ErrEventNotFound, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tenantServer := newTenantServer(t, &fakeTenantService{event: tt.event, authorizations: make(chan string, 1)})

			event, err := NewTenantClient(tenantServer.Client(), tenantServer.URL).FindEvent(context.Background(), "fedcba9876543210")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FindEvent() error = %v, want %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if event.PublicID() != "fedcba9876543210" || event.TenantPublicID() != "a1b2c3d4e5f60718" || event.Archived() != tt.wantArchived {
				t.Errorf("FindEvent() = {%q %q archived=%v}, want {fedcba9876543210 a1b2c3d4e5f60718 archived=%v}",
					event.PublicID(), event.TenantPublicID(), event.Archived(), tt.wantArchived)
			}
		})
	}
}
