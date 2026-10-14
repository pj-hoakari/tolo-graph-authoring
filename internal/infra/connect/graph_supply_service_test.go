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
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
	kernelv1 "github.com/pj-hoakari/tolo-kernel-proto/gen/tolo/kernel/v1"
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
			Kernel: domain.GraphDocument{
				Nodes: []domain.Node{
					{ID: "gate", Type: domain.NodeTypeTransitOnly},
					{ID: "hall", Type: domain.NodeTypeGoalTransitMixed},
					{ID: "outside", Type: domain.NodeTypeExternal},
				},
				Edges: []domain.Edge{
					{ID: "e1", SourceNodeID: "gate", TargetNodeID: "hall", Direction: domain.EdgeDirectionBothWays},
					{ID: "entry", SourceNodeID: "outside", TargetNodeID: "gate", Direction: domain.EdgeDirectionOneWay},
				},
			}.Parts().Kernel,
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
			{
				PointId: "gate", Type: kernelv1.PointType_POINT_TYPE_TRANSIT_ONLY,
				Boundary: &kernelv1.Boundary{Direction: kernelv1.BoundaryDirection_BOUNDARY_DIRECTION_ENTRY, Active: true},
			},
			{PointId: "hall", Type: kernelv1.PointType_POINT_TYPE_GOAL_TRANSIT_MIXED, Boundary: nil},
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

type mappedRevisionRepository struct {
	nopGraphRepository

	revision domain.PublishedRevision
}

func (r mappedRevisionRepository) FindRevision(_ context.Context, eventPublicID, revisionID string) (domain.PublishedRevision, error) {
	if eventPublicID != r.revision.EventPublicID || revisionID != r.revision.RevisionID {
		return domain.PublishedRevision{}, repository.ErrRevisionNotFound
	}

	return r.revision, nil
}

func TestGetObservationPointMappingsServesMappingsOfTheRequestedRevisionToServiceToken(t *testing.T) {
	t.Parallel()

	position := 0.25
	authorization, keys := mintInternalJWT(t, internaljwt.TokenUseService, "", "")
	client := newSupplyClient(t, keys, application.NewGraphService(mappedRevisionRepository{
		nopGraphRepository: nopGraphRepository{},
		revision: domain.PublishedRevision{
			TenantPublicID: "a1b2c3d4e5f60718",
			EventPublicID:  "fedcba9876543210",
			RevisionID:     "0123456789abcdef",
			Placements: domain.Placements{Mappings: []domain.ObservationPointMapping{
				{ObservationPointID: "cam-1", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindRoute, ElementID: "e1", RoutePosition: &position}},
				{ObservationPointID: "cam-2", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "gate"}},
			}},
		},
	}, inlineTransactor{}, callerTenantEvents{}))

	getMappings := func(revisionID string) (*connectrpc.Response[graphv1.GetMappingsResponse], error) {
		req := connectrpc.NewRequest(&graphv1.GetMappingsRequest{EventId: "fedcba9876543210", RevisionId: revisionID})
		req.Header().Set("Authorization", authorization)

		return client.GetObservationPointMappings(context.Background(), req)
	}

	res, err := getMappings("0123456789abcdef")
	if err != nil {
		t.Fatalf("GetObservationPointMappings() error = %v", err)
	}

	want := &graphv1.GetMappingsResponse{
		RevisionId: "0123456789abcdef",
		Mappings: []*graphv1.ObservationPointMapping{
			{ObservationPointId: "cam-1", Anchor: &kernelv1.GraphAnchor{Target: &kernelv1.GraphAnchor_RouteId{RouteId: "e1"}, RoutePosition: proto.Float64(0.25)}},
			{ObservationPointId: "cam-2", Anchor: &kernelv1.GraphAnchor{Target: &kernelv1.GraphAnchor_PointId{PointId: "gate"}}},
		},
	}

	if !proto.Equal(res.Msg, want) {
		t.Errorf("GetObservationPointMappings() = %v, want %v", res.Msg, want)
	}

	for _, tc := range []struct {
		name       string
		revisionID string
		want       connectrpc.Code
	}{
		{"rejects a missing revision ID", "", connectrpc.CodeInvalidArgument},
		{"answers an unknown revision with not found", "fedcba9876543210", connectrpc.CodeNotFound},
	} {
		if _, err := getMappings(tc.revisionID); connectrpc.CodeOf(err) != tc.want {
			t.Errorf("%s: GetObservationPointMappings() error = %v, want code %v", tc.name, err, tc.want)
		}
	}
}
