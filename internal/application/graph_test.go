//go:generate go tool mockgen -source=../repository/graph.go -destination=mock_graph_repository_test.go -package=application_test

package application_test

import (
	"context"
	"errors"
	"math"
	"regexp"
	"testing"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/tenantctx"
	"go.uber.org/mock/gomock"
)

const (
	tenantID = "a1b2c3d4e5f60718"
	eventID  = "fedcba9876543210"
)

var revisionIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

func eventContext(tenantPublicID, eventPublicID string) context.Context {
	return internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
		TokenUse:       internaljwt.TokenUseEventAccess,
		TenantPublicID: tenantPublicID,
		EventPublicID:  eventPublicID,
	})
}

func document(nodeID string) *domain.GraphDocument {
	return &domain.GraphDocument{Nodes: []domain.Node{{ID: nodeID, Type: domain.NodeTypeGoal}}}
}

func venueGraph(t *testing.T, tenantPublicID string, draft *domain.GraphDocument) domain.VenueGraph {
	t.Helper()

	graph, err := domain.NewVenueGraph(tenantPublicID, eventID, *draft)
	if err != nil {
		t.Fatalf("NewVenueGraph() error = %v", err)
	}

	return graph
}

func saveNewGraph(t *testing.T, draft *domain.GraphDocument) domain.VenueGraph {
	t.Helper()

	graphs := NewMockGraphRepository(gomock.NewController(t))
	graphs.EXPECT().FindByEventPublicID(gomock.Any(), eventID).Return(domain.VenueGraph{}, repository.ErrGraphNotFound)
	graphs.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)

	graph, err := application.NewGraphService(graphs).SaveGraph(eventContext(tenantID, eventID), application.SaveGraphInput{
		EventPublicID: eventID,
		Document:      draft,
	})
	if err != nil {
		t.Fatalf("SaveGraph() error = %v", err)
	}

	return graph
}

func TestSaveGraphDerivesDraftRevisionIDFromDocument(t *testing.T) {
	t.Parallel()

	first := saveNewGraph(t, document("n1")).DraftRevisionID()
	if !revisionIDPattern.MatchString(first) {
		t.Errorf("DraftRevisionID() = %q, want 16 lowercase hex characters", first)
	}

	if again := saveNewGraph(t, document("n1")).DraftRevisionID(); again != first {
		t.Errorf("DraftRevisionID() for the same document = %q, want %q", again, first)
	}

	if other := saveNewGraph(t, document("n2")).DraftRevisionID(); other == first {
		t.Errorf("DraftRevisionID() for a different document = %q, want it to differ from %q", other, first)
	}
}

func TestSaveGraphKeepsDraftRevisionIDWhenResavingSameDocument(t *testing.T) {
	t.Parallel()

	existing := venueGraph(t, tenantID, document("n1"))

	graphs := NewMockGraphRepository(gomock.NewController(t))
	graphs.EXPECT().FindByEventPublicID(gomock.Any(), eventID).Return(existing, nil)
	graphs.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)

	graph, err := application.NewGraphService(graphs).SaveGraph(eventContext(tenantID, eventID), application.SaveGraphInput{
		EventPublicID: eventID,
		Document:      document("n1"),
	})
	if err != nil {
		t.Fatalf("SaveGraph() error = %v", err)
	}

	if graph.DraftRevisionID() != existing.DraftRevisionID() {
		t.Errorf("DraftRevisionID() = %q, want the existing %q", graph.DraftRevisionID(), existing.DraftRevisionID())
	}
}

func TestSaveGraphRejectsUnencodableLayoutWithoutSaving(t *testing.T) {
	t.Parallel()

	graphs := NewMockGraphRepository(gomock.NewController(t))
	graphs.EXPECT().FindByEventPublicID(gomock.Any(), eventID).Return(domain.VenueGraph{}, repository.ErrGraphNotFound)

	draft := document("n1")
	draft.Nodes[0].Layout.X = math.NaN()

	_, err := application.NewGraphService(graphs).SaveGraph(eventContext(tenantID, eventID), application.SaveGraphInput{
		EventPublicID: eventID,
		Document:      draft,
	})
	if !errors.Is(err, domain.ErrInvalidGraphDocument) {
		t.Errorf("SaveGraph() error = %v, want %v", err, domain.ErrInvalidGraphDocument)
	}
}

func TestSaveGraphCreatesGraphOnFirstSave(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	graphs := NewMockGraphRepository(ctrl)
	graphs.EXPECT().FindByEventPublicID(gomock.Any(), eventID).Return(domain.VenueGraph{}, repository.ErrGraphNotFound)

	var saved domain.VenueGraph

	graphs.EXPECT().Save(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, graph domain.VenueGraph) error {
		saved = graph

		return nil
	})

	graph, err := application.NewGraphService(graphs).SaveGraph(eventContext(tenantID, eventID), application.SaveGraphInput{
		EventPublicID: eventID,
		Document:      document("n1"),
	})
	if err != nil {
		t.Fatalf("SaveGraph() error = %v", err)
	}

	if saved.TenantPublicID() != tenantID || saved.EventPublicID() != eventID || saved.Draft().Nodes[0].ID != "n1" {
		t.Errorf("saved graph = %+v, want a new graph for tenant %s event %s holding the document", saved, tenantID, eventID)
	}

	if !revisionIDPattern.MatchString(saved.DraftRevisionID()) {
		t.Errorf("DraftRevisionID() = %q, want 16 hex characters", saved.DraftRevisionID())
	}

	if saved.RevisionID() != "" {
		t.Errorf("RevisionID() = %q, want empty before publishing", saved.RevisionID())
	}

	if graph.DraftRevisionID() != saved.DraftRevisionID() {
		t.Errorf("returned DraftRevisionID() = %q, want the saved %q", graph.DraftRevisionID(), saved.DraftRevisionID())
	}
}

func TestSaveGraphReplacesDraftOfExistingGraph(t *testing.T) {
	t.Parallel()

	existing := venueGraph(t, tenantID, document("old"))

	ctrl := gomock.NewController(t)
	graphs := NewMockGraphRepository(ctrl)
	graphs.EXPECT().FindByEventPublicID(gomock.Any(), eventID).Return(existing, nil)

	var saved domain.VenueGraph

	graphs.EXPECT().Save(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, graph domain.VenueGraph) error {
		saved = graph

		return nil
	})

	_, err := application.NewGraphService(graphs).SaveGraph(eventContext(tenantID, eventID), application.SaveGraphInput{
		EventPublicID: eventID,
		Document:      document("new"),
	})
	if err != nil {
		t.Fatalf("SaveGraph() error = %v", err)
	}

	if saved.TenantPublicID() != tenantID || saved.EventPublicID() != eventID {
		t.Errorf("saved graph owner = %s/%s, want %s/%s", saved.TenantPublicID(), saved.EventPublicID(), tenantID, eventID)
	}

	if got := saved.Draft().Nodes[0].ID; got != "new" {
		t.Errorf("saved draft node = %q, want %q", got, "new")
	}

	if id := saved.DraftRevisionID(); id == existing.DraftRevisionID() || !revisionIDPattern.MatchString(id) {
		t.Errorf("DraftRevisionID() = %q, want a 16 hex character ID that differs from the old draft's", id)
	}
}

func TestSaveGraphRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input application.SaveGraphInput
		want  error
	}{
		{"missing event ID", application.SaveGraphInput{EventPublicID: "", Document: document("n1")}, application.ErrEventIDRequired},
		{"missing document", application.SaveGraphInput{EventPublicID: eventID, Document: nil}, application.ErrGraphDocumentRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := application.NewGraphService(NewMockGraphRepository(gomock.NewController(t)))

			_, err := service.SaveGraph(eventContext(tenantID, eventID), tt.input)
			if !errors.Is(err, tt.want) {
				t.Errorf("SaveGraph() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSaveGraphRejectsUnauthorizedContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"other event", eventContext(tenantID, "0123456789abcdef"), tenantctx.ErrEventMismatch},
		{"no event claim", eventContext(tenantID, ""), tenantctx.ErrEventMissing},
		{"no tenant claim", eventContext("", eventID), tenantctx.ErrMissing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := application.NewGraphService(NewMockGraphRepository(gomock.NewController(t)))

			_, err := service.SaveGraph(tt.ctx, application.SaveGraphInput{EventPublicID: eventID, Document: document("n1")})
			if !errors.Is(err, tt.want) {
				t.Errorf("SaveGraph() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSaveGraphRejectsGraphOfAnotherTenant(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	graphs := NewMockGraphRepository(ctrl)
	graphs.EXPECT().FindByEventPublicID(gomock.Any(), eventID).
		Return(venueGraph(t, "ffffffffffffffff", document("old")), nil)

	_, err := application.NewGraphService(graphs).SaveGraph(eventContext(tenantID, eventID), application.SaveGraphInput{
		EventPublicID: eventID,
		Document:      document("new"),
	})
	if !errors.Is(err, tenantctx.ErrMismatch) {
		t.Errorf("SaveGraph() error = %v, want %v", err, tenantctx.ErrMismatch)
	}
}

func TestSaveGraphPropagatesRepositoryErrors(t *testing.T) {
	t.Parallel()

	errStore := errors.New("store unavailable")

	t.Run("find", func(t *testing.T) {
		t.Parallel()

		graphs := NewMockGraphRepository(gomock.NewController(t))
		graphs.EXPECT().FindByEventPublicID(gomock.Any(), eventID).Return(domain.VenueGraph{}, errStore)

		_, err := application.NewGraphService(graphs).SaveGraph(eventContext(tenantID, eventID), application.SaveGraphInput{EventPublicID: eventID, Document: document("n1")})
		if !errors.Is(err, errStore) {
			t.Errorf("SaveGraph() error = %v, want %v", err, errStore)
		}
	})

	t.Run("save", func(t *testing.T) {
		t.Parallel()

		graphs := NewMockGraphRepository(gomock.NewController(t))
		graphs.EXPECT().FindByEventPublicID(gomock.Any(), eventID).Return(domain.VenueGraph{}, repository.ErrGraphNotFound)
		graphs.EXPECT().Save(gomock.Any(), gomock.Any()).Return(errStore)

		_, err := application.NewGraphService(graphs).SaveGraph(eventContext(tenantID, eventID), application.SaveGraphInput{EventPublicID: eventID, Document: document("n1")})
		if !errors.Is(err, errStore) {
			t.Errorf("SaveGraph() error = %v, want %v", err, errStore)
		}
	})
}
