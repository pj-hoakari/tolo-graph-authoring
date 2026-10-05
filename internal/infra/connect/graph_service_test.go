package connect

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/internal-jwt-handling/jwtgen"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/infra/memory"
)

func newTestGraphService() *application.GraphService {
	return application.NewGraphService(memory.NewGraphRepository())
}

func mintEventAccessJWT(t *testing.T, tenantPublicID, eventPublicID string) (string, internaljwt.JWKS) {
	t.Helper()

	output, err := jwtgen.Generate(jwtgen.Config{
		Issuer:         DefaultInternalJWTIssuer,
		Audience:       DefaultInternalJWTAudience,
		TokenUse:       internaljwt.TokenUseEventAccess,
		TenantPublicID: tenantPublicID,
		EventPublicID:  eventPublicID,
		Scope:          "events.manage",
		KeyID:          "test-key",
		TTL:            time.Hour,
	})
	if err != nil {
		t.Fatalf("generate internal JWT: %v", err)
	}

	return "Bearer " + output.Token, output.JWKS
}

func TestSaveGraph(t *testing.T) {
	t.Parallel()

	authorization, keys := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")
	httpServer := httptest.NewServer(newTestHandler(t, keys, application.NewGreetService(nopGreetingRepository{})))
	t.Cleanup(httpServer.Close)
	client := graphv1connect.NewGraphAuthoringServiceClient(httpServer.Client(), httpServer.URL)

	save := func(eventID string) (*connectrpc.Response[graphv1.GraphMeta], error) {
		req := connectrpc.NewRequest(&graphv1.SaveGraphRequest{
			EventId: eventID,
			Document: &graphv1.GraphDocument{
				Nodes: []*graphv1.GraphNode{{NodeId: "n1", NodeType: graphv1.NodeType_NODE_TYPE_GOAL}},
			},
		})
		req.Header().Set("Authorization", authorization)

		return client.SaveGraph(context.Background(), req)
	}

	t.Run("saves the draft of the token's event", func(t *testing.T) {
		t.Parallel()

		res, err := save("fedcba9876543210")
		if err != nil {
			t.Fatalf("SaveGraph() error = %v", err)
		}

		if got, want := res.Msg.GetEventId(), "fedcba9876543210"; got != want {
			t.Errorf("EventId = %q, want %q", got, want)
		}

		if res.Msg.GetDraftRevisionId() == "" {
			t.Error("DraftRevisionId is empty, want the new draft revision")
		}
	})

	t.Run("rejects another event", func(t *testing.T) {
		t.Parallel()

		_, err := save("0123456789abcdef")
		if got, want := connectrpc.CodeOf(err), connectrpc.CodePermissionDenied; got != want {
			t.Fatalf("SaveGraph() error code = %v, want %v", got, want)
		}
	})
}
