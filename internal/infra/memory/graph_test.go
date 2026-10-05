package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/infra/memory"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
)

func TestGraphRepositorySaveThenFind(t *testing.T) {
	t.Parallel()

	repo := memory.NewGraphRepository()

	graph, err := domain.NewVenueGraph("a1b2c3d4e5f60718", "fedcba9876543210", domain.GraphDocument{
		Nodes: []domain.Node{{ID: "n1", Type: domain.NodeTypeGoal}},
	})
	if err != nil {
		t.Fatalf("NewVenueGraph() error = %v", err)
	}

	if err := repo.Save(context.Background(), graph); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := repo.FindByEventPublicID(context.Background(), "fedcba9876543210")
	if err != nil {
		t.Fatalf("FindByEventPublicID() error = %v", err)
	}

	if got.TenantPublicID() != "a1b2c3d4e5f60718" || got.DraftRevisionID() != graph.DraftRevisionID() || got.Draft().Nodes[0].ID != "n1" {
		t.Errorf("FindByEventPublicID() = %+v, want the saved graph", got)
	}
}

func TestGraphRepositoryFindUnknown(t *testing.T) {
	t.Parallel()

	_, err := memory.NewGraphRepository().FindByEventPublicID(context.Background(), "fedcba9876543210")
	if !errors.Is(err, repository.ErrGraphNotFound) {
		t.Errorf("FindByEventPublicID() error = %v, want %v", err, repository.ErrGraphNotFound)
	}
}
