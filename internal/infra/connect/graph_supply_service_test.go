package connect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connectrpc "connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
	kernelv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/kernel/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

type currentRevisionRepository struct {
	nopGraphRepository

	revision domain.PublishedRevision
}

func (r currentRevisionRepository) FindCurrentRevision(context.Context, string) (domain.PublishedRevision, error) {
	return r.revision, nil
}

func newSupplyClient(t *testing.T, keys internaljwt.JWKS, graphs application.GraphUseCases) graphv1connect.GraphSupplyServiceClient {
	t.Helper()

	routes, err := RoutesWithVerifier(graphs, newTestVerifier(t, keys))
	if err != nil {
		t.Fatalf("RoutesWithVerifier() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	return graphv1connect.NewGraphSupplyServiceClient(httpServer.Client(), httpServer.URL)
}

func getCurrentRevision(client graphv1connect.GraphSupplyServiceClient, authorization string) (*connectrpc.Response[kernelv1.Graph], error) {
	req := connectrpc.NewRequest(&graphv1.GetCurrentRevisionRequest{EventId: "fedcba9876543210"})
	req.Header().Set("Authorization", authorization)

	return client.GetCurrentRevision(context.Background(), req)
}

func TestGetCurrentRevisionServesKernelGraphToServiceToken(t *testing.T) {
	t.Parallel()

	authorization, keys := mintInternalJWT(t, internaljwt.TokenUseService, "", "")
	client := newSupplyClient(t, keys, application.NewGraphService(currentRevisionRepository{
		nopGraphRepository: nopGraphRepository{},
		revision: domain.PublishedRevision{
			TenantPublicID: "a1b2c3d4e5f60718",
			EventPublicID:  "fedcba9876543210",
			RevisionID:     "0123456789abcdef",
			Document: domain.GraphDocument{
				Nodes: []domain.Node{
					{ID: "gate", Type: domain.NodeTypeBoundary},
					{ID: "hall", Type: domain.NodeTypeGoalTransitMixed},
				},
				Edges: []domain.Edge{
					{ID: "e1", SourceNodeID: "gate", TargetNodeID: "hall", Direction: domain.EdgeDirectionBothWays},
				},
			},
		},
	}, inlineTransactor{}, callerTenantEvents{}))

	res, err := getCurrentRevision(client, authorization)
	if err != nil {
		t.Fatalf("GetCurrentRevision() error = %v", err)
	}

	want := &kernelv1.Graph{
		EventId:    "fedcba9876543210",
		RevisionId: "0123456789abcdef",
		Points: []*kernelv1.Point{
			{PointId: "gate", Type: kernelv1.PointType_POINT_TYPE_TRANSIT_ONLY, IsBoundary: true, BoundaryActive: true},
			{PointId: "hall", Type: kernelv1.PointType_POINT_TYPE_GOAL_TRANSIT_MIXED, IsBoundary: false, BoundaryActive: true},
		},
		Routes: []*kernelv1.Route{
			{RouteId: "e1", FromPointId: "gate", ToPointId: "hall", Direction: kernelv1.DirectionAttribute_DIRECTION_ATTRIBUTE_BOTH_WAYS},
		},
	}

	if !proto.Equal(res.Msg, want) {
		t.Errorf("GetCurrentRevision() = %v, want %v", res.Msg, want)
	}
}

func TestGetCurrentRevisionAnswersUnpublishedEventWithNotFound(t *testing.T) {
	t.Parallel()

	authorization, keys := mintInternalJWT(t, internaljwt.TokenUseService, "", "")

	_, err := getCurrentRevision(newSupplyClient(t, keys, newTestGraphService()), authorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeNotFound; got != want {
		t.Errorf("GetCurrentRevision() error code = %v, want %v", got, want)
	}
}

func TestGetCurrentRevisionRejectsEventAccessToken(t *testing.T) {
	t.Parallel()

	authorization, keys := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")

	_, err := getCurrentRevision(newSupplyClient(t, keys, newTestGraphService()), authorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Errorf("GetCurrentRevision() error code = %v, want %v", got, want)
	}
}
