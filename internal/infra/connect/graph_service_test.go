package connect

import (
	"context"
	"errors"
	"net/http/httptest"
	"reflect"
	"testing"

	connectrpc "connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	tenantv1 "github.com/pj-hoakari/tolo-tenant-management/gen/tolo/tenant/v1"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
	kernelv1 "github.com/pj-hoakari/tolo-kernel-proto/gen/tolo/kernel/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	dbinfra "github.com/pj-hoakari/tolo-graph-authoring/internal/infra/db"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/tenantctx"
)

type nopGraphRepository struct{}

func (nopGraphRepository) FindByEventPublicID(context.Context, string, string) (domain.Graph, error) {
	return domain.Graph{}, repository.ErrGraphNotFound
}

func (nopGraphRepository) FindByEventPublicIDForUpdate(context.Context, string, string) (domain.Graph, error) {
	return domain.Graph{}, repository.ErrGraphNotFound
}

func (nopGraphRepository) Save(context.Context, domain.Graph) error { return nil }

func (nopGraphRepository) Publish(context.Context, domain.Graph) error { return nil }

func (nopGraphRepository) FindCurrentRevision(context.Context, string) (domain.PublishedRevision, error) {
	return domain.PublishedRevision{}, repository.ErrGraphNotFound
}

func (nopGraphRepository) SaveObservationPointMapping(context.Context, domain.Graph, domain.ObservationPointMapping) error {
	return nil
}

func (nopGraphRepository) FindObservationPointMappings(context.Context, string) ([]domain.ObservationPointMapping, error) {
	return nil, nil
}

func (nopGraphRepository) FindPlacements(context.Context, domain.Graph) (domain.Placements, error) {
	return domain.Placements{}, nil
}

func (nopGraphRepository) AddQrLocation(context.Context, domain.Graph, domain.QrLocation) error {
	return nil
}

func (nopGraphRepository) UpdateQrLocation(context.Context, domain.Graph, domain.QrLocation) error {
	return nil
}

func (nopGraphRepository) RemoveQrLocation(context.Context, domain.Graph, string) error { return nil }

type inlineTransactor struct{}

func (inlineTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type callerTenantEvents struct{}

func (callerTenantEvents) FindEvent(ctx context.Context, eventPublicID string) (domain.Event, error) {
	tenantPublicID, _ := tenantctx.TenantPublicIDFromContext(ctx)

	return domain.NewEvent(eventPublicID, tenantPublicID, false), nil
}

func newTestGraphService() *application.GraphService {
	return application.NewGraphService(nopGraphRepository{}, inlineTransactor{}, callerTenantEvents{})
}

func TestSaveGraph(t *testing.T) {
	t.Parallel()

	authorization, keys := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")
	httpServer := httptest.NewServer(newTestHandler(t, keys, newTestGraphService()))
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

	t.Run("returns the same meta when the same document is saved again", func(t *testing.T) {
		t.Parallel()

		first, err := save("fedcba9876543210")
		if err != nil {
			t.Fatalf("first SaveGraph() error = %v", err)
		}

		second, err := save("fedcba9876543210")
		if err != nil {
			t.Fatalf("second SaveGraph() error = %v", err)
		}

		if !proto.Equal(first.Msg, second.Msg) {
			t.Errorf("second GraphMeta = %v, want %v", second.Msg, first.Msg)
		}
	})

	t.Run("rejects another event", func(t *testing.T) {
		t.Parallel()

		_, err := save("0123456789abcdef")
		if got, want := connectrpc.CodeOf(err), connectrpc.CodePermissionDenied; got != want {
			t.Fatalf("SaveGraph() error code = %v, want %v", got, want)
		}
	})

	t.Run("rejects a structurally invalid document", func(t *testing.T) {
		t.Parallel()

		req := connectrpc.NewRequest(&graphv1.SaveGraphRequest{
			EventId: "fedcba9876543210",
			Document: &graphv1.GraphDocument{
				Nodes:  []*graphv1.GraphNode{{NodeId: "outside", NodeType: graphv1.NodeType_NODE_TYPE_EXTERNAL, GroupId: "floor"}},
				Groups: []*graphv1.NodeGroup{{GroupId: "floor"}},
			},
		})
		req.Header().Set("Authorization", authorization)

		_, err := client.SaveGraph(context.Background(), req)
		if got, want := connectrpc.CodeOf(err), connectrpc.CodeInvalidArgument; got != want {
			t.Fatalf("SaveGraph() error code = %v, want %v", got, want)
		}
	})
}

func TestGraphDocumentFromProtoKeepsExternalNodesAndNestedGroups(t *testing.T) {
	t.Parallel()

	size := func(v float64) *float64 { return &v }

	got := graphDocumentFromProto(&graphv1.GraphDocument{
		Nodes: []*graphv1.GraphNode{
			{NodeId: "outside", NodeType: graphv1.NodeType_NODE_TYPE_EXTERNAL, Layout: &graphv1.Layout{X: -10, Y: 0}},
			{NodeId: "gate", NodeType: graphv1.NodeType_NODE_TYPE_TRANSIT_ONLY, GroupId: "hall"},
		},
		Groups: []*graphv1.NodeGroup{
			{GroupId: "floor", MinWidth: size(300), MinHeight: size(200)},
			{GroupId: "hall", ParentGroupId: "floor"},
		},
	})

	want := &domain.GraphDocument{
		Nodes: []domain.Node{
			{ID: "outside", Type: domain.NodeTypeExternal, Layout: domain.Layout{X: -10, Y: 0}},
			{ID: "gate", Type: domain.NodeTypeTransitOnly, GroupID: "hall"},
		},
		Groups: []domain.Group{
			{ID: "floor", MinWidth: size(300), MinHeight: size(200)},
			{ID: "hall", ParentGroupID: "floor"},
		},
		Edges: []domain.Edge{},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("graphDocumentFromProto() = %+v, want %+v", got, want)
	}
}

func TestPublishRevisionAnswersMissingDraftWithNotFound(t *testing.T) {
	t.Parallel()

	authorization, keys := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")
	httpServer := httptest.NewServer(newTestHandler(t, keys, newTestGraphService()))
	t.Cleanup(httpServer.Close)
	client := graphv1connect.NewGraphAuthoringServiceClient(httpServer.Client(), httpServer.URL)

	req := connectrpc.NewRequest(&graphv1.PublishRevisionRequest{EventId: "fedcba9876543210"})
	req.Header().Set("Authorization", authorization)

	_, err := client.PublishRevision(context.Background(), req)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeNotFound; got != want {
		t.Fatalf("PublishRevision() error code = %v, want %v", got, want)
	}
}

func TestGraphErrorAnswersAbortedTransactionWithAborted(t *testing.T) {
	t.Parallel()

	err := errors.Join(errors.New("deadlock detected"), dbinfra.ErrTransactionAborted)
	if got, want := graphError(context.Background(), err).Code(), connectrpc.CodeAborted; got != want {
		t.Errorf("graphError() code = %v, want %v", got, want)
	}
}

func TestSaveGraphVerifiesEventWithTenantManagement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event *tenantv1.Event
		err   error
		want  connectrpc.Code
	}{
		{"open event", tenantEvent(tenantv1.EventStatus_EVENT_STATUS_OPEN), nil, 0},
		{"unknown event", nil, nil, connectrpc.CodeFailedPrecondition},
		{"archived event", tenantEvent(tenantv1.EventStatus_EVENT_STATUS_ARCHIVED), nil, connectrpc.CodeFailedPrecondition},
		{"tenant management unavailable", nil, connectrpc.NewError(connectrpc.CodeUnavailable, errors.New("down")), connectrpc.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := &fakeTenantService{event: tt.event, err: tt.err, authorizations: make(chan string, 1)}
			tenantServer := newTenantServer(t, service)

			graphs := application.NewGraphService(nopGraphRepository{}, inlineTransactor{}, NewTenantClient(tenantServer.Client(), tenantServer.URL))
			authorization, keys := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")

			_, err := saveGraphThrough(t, newTestHandler(t, keys, graphs), authorization)
			if got := connectrpc.CodeOf(err); err != nil && got != tt.want || err == nil && tt.want != 0 {
				t.Fatalf("SaveGraph() error = %v, want code %v", err, tt.want)
			}

			select {
			case got := <-service.authorizations:
				if got != authorization {
					t.Errorf("Authorization sent to tenant management = %q, want the caller's %q", got, authorization)
				}
			default:
				t.Error("tenant management was not consulted")
			}
		})
	}
}

type draftGraphRepository struct {
	nopGraphRepository

	draft domain.GraphDocument
}

func (r draftGraphRepository) FindByEventPublicIDForUpdate(_ context.Context, tenantPublicID, eventPublicID string) (domain.Graph, error) {
	return domain.NewGraph(tenantPublicID, eventPublicID, r.draft)
}

func TestMapObservationPoint(t *testing.T) {
	t.Parallel()

	authorization, keys := mintEventAccessJWT(t, "a1b2c3d4e5f60718", "fedcba9876543210")
	graphs := application.NewGraphService(draftGraphRepository{
		nopGraphRepository: nopGraphRepository{},
		draft: domain.GraphDocument{
			Nodes: []domain.Node{{ID: "gate", Type: domain.NodeTypeTransitOnly}, {ID: "hall", Type: domain.NodeTypeGoal}},
			Edges: []domain.Edge{{ID: "e1", SourceNodeID: "gate", TargetNodeID: "hall", Direction: domain.EdgeDirectionOneWay}},
		},
	}, inlineTransactor{}, callerTenantEvents{})
	httpServer := httptest.NewServer(newTestHandler(t, keys, graphs))
	t.Cleanup(httpServer.Close)
	client := graphv1connect.NewGraphAuthoringServiceClient(httpServer.Client(), httpServer.URL)

	mapPoint := func(mapping *graphv1.ObservationPointMapping) (*connectrpc.Response[graphv1.ObservationPointMapping], error) {
		req := connectrpc.NewRequest(&graphv1.MapObservationPointRequest{EventId: "fedcba9876543210", Mapping: mapping})
		req.Header().Set("Authorization", authorization)

		return client.MapObservationPoint(context.Background(), req)
	}

	t.Run("answers with the mapping onto a route", func(t *testing.T) {
		t.Parallel()

		mapping := &graphv1.ObservationPointMapping{
			ObservationPointId: "cam-1",
			Anchor:             &kernelv1.GraphAnchor{Target: &kernelv1.GraphAnchor_RouteId{RouteId: "e1"}, RoutePosition: proto.Float64(0.5)},
		}

		res, err := mapPoint(mapping)
		if err != nil {
			t.Fatalf("MapObservationPoint() error = %v", err)
		}

		if !proto.Equal(res.Msg, mapping) {
			t.Errorf("MapObservationPoint() = %v, want %v", res.Msg, mapping)
		}
	})

	for _, tc := range []struct {
		name    string
		mapping *graphv1.ObservationPointMapping
		want    connectrpc.Code
	}{
		{"rejects a mapping without anchor", &graphv1.ObservationPointMapping{ObservationPointId: "cam-1"}, connectrpc.CodeInvalidArgument},
		{"rejects a missing mapping", nil, connectrpc.CodeInvalidArgument},
		{
			"rejects a point missing from the draft",
			&graphv1.ObservationPointMapping{ObservationPointId: "cam-1", Anchor: &kernelv1.GraphAnchor{Target: &kernelv1.GraphAnchor_PointId{PointId: "nowhere"}}},
			connectrpc.CodeFailedPrecondition,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := mapPoint(tc.mapping)
			if got := connectrpc.CodeOf(err); got != tc.want {
				t.Errorf("MapObservationPoint() error code = %v, want %v", got, tc.want)
			}
		})
	}
}
