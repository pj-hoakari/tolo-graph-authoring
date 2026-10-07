package connect

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	connectrpc "connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	dbinfra "github.com/pj-hoakari/tolo-graph-authoring/internal/infra/db"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
)

type nopGraphRepository struct{}

func (nopGraphRepository) FindByEventPublicIDForUpdate(context.Context, string, string) (domain.Graph, error) {
	return domain.Graph{}, repository.ErrGraphNotFound
}

func (nopGraphRepository) Save(context.Context, domain.Graph) error { return nil }

func (nopGraphRepository) Publish(context.Context, domain.Graph) error { return nil }

func (nopGraphRepository) FindCurrentRevision(context.Context, string) (domain.PublishedRevision, error) {
	return domain.PublishedRevision{}, repository.ErrGraphNotFound
}

type inlineTransactor struct{}

func (inlineTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func newTestGraphService() *application.GraphService {
	return application.NewGraphService(nopGraphRepository{}, inlineTransactor{})
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
