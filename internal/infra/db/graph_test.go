package db

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/tenantctx"
)

const (
	ownerTenant = "a1b2c3d4e5f60718"
	otherTenant = "ffffffffffffffff"
	graphEvent  = "fedcba9876543210"
)

func newTestGraphRepository(t *testing.T) *PostgresGraphRepository {
	t.Helper()

	if _, err := testDB.Exec(`TRUNCATE graph_drafts, graphs`); err != nil {
		t.Fatalf("truncate graph tables: %v", err)
	}

	return NewPostgresGraphRepository(testDB)
}

func newGraph(t *testing.T, tenantPublicID string, document domain.GraphDocument) domain.VenueGraph {
	t.Helper()

	graph, err := domain.NewVenueGraph(tenantPublicID, graphEvent, document)
	if err != nil {
		t.Fatalf("NewVenueGraph() error = %v", err)
	}

	return graph
}

func singleNode(id string) domain.GraphDocument {
	return domain.GraphDocument{Nodes: []domain.Node{{ID: id, Type: domain.NodeTypeGoal}}}
}

func assertDraft(t *testing.T, repo *PostgresGraphRepository, want domain.VenueGraph) {
	t.Helper()

	got, err := repo.FindByEventPublicIDForUpdate(context.Background(), want.TenantPublicID(), want.EventPublicID())
	if err != nil {
		t.Fatalf("FindByEventPublicIDForUpdate() error = %v", err)
	}

	if !reflect.DeepEqual(got.Draft(), want.Draft()) {
		t.Errorf("loaded draft = %#v, want %#v", got.Draft(), want.Draft())
	}

	if got.TenantPublicID() != want.TenantPublicID() || got.EventPublicID() != want.EventPublicID() {
		t.Errorf("loaded owner = %s/%s, want %s/%s", got.TenantPublicID(), got.EventPublicID(), want.TenantPublicID(), want.EventPublicID())
	}

	if got.DraftRevisionID() != want.DraftRevisionID() {
		t.Errorf("loaded DraftRevisionID() = %q, want %q", got.DraftRevisionID(), want.DraftRevisionID())
	}

	if got.RevisionID() != "" {
		t.Errorf("loaded RevisionID() = %q, want empty", got.RevisionID())
	}
}

func TestPostgresGraphRepositorySaveThenFind(t *testing.T) {
	repo := newTestGraphRepository(t)
	graph := newGraph(t, ownerTenant, richDocument())

	if err := repo.Save(context.Background(), graph); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	assertDraft(t, repo, graph)

	var revisionID string
	if err := testDB.Get(&revisionID, `SELECT revision_id FROM graph_drafts WHERE event_public_id = $1`, graphEvent); err != nil {
		t.Fatalf("select stored revision ID: %v", err)
	}

	if revisionID != graph.DraftRevisionID() {
		t.Errorf("stored revision_id = %q, want %q", revisionID, graph.DraftRevisionID())
	}
}

func TestPostgresGraphRepositorySaveOverwritesDraft(t *testing.T) {
	repo := newTestGraphRepository(t)
	ctx := context.Background()

	if err := repo.Save(ctx, newGraph(t, ownerTenant, singleNode("old"))); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}

	second := newGraph(t, ownerTenant, singleNode("new"))
	if err := repo.Save(ctx, second); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	assertDraft(t, repo, second)

	var drafts int
	if err := testDB.Get(&drafts, `SELECT COUNT(*) FROM graph_drafts`); err != nil {
		t.Fatalf("count drafts: %v", err)
	}

	if drafts != 1 {
		t.Errorf("draft rows = %d, want 1", drafts)
	}
}

func TestPostgresGraphRepositorySaveRejectsOtherTenant(t *testing.T) {
	repo := newTestGraphRepository(t)
	ctx := context.Background()

	owned := newGraph(t, ownerTenant, singleNode("owner"))
	if err := repo.Save(ctx, owned); err != nil {
		t.Fatalf("owner Save() error = %v", err)
	}

	if err := repo.Save(ctx, newGraph(t, otherTenant, singleNode("intruder"))); !errors.Is(err, tenantctx.ErrMismatch) {
		t.Fatalf("other tenant Save() error = %v, want %v", err, tenantctx.ErrMismatch)
	}

	assertDraft(t, repo, owned)
}

func TestPostgresGraphRepositoryFindScopesByTenant(t *testing.T) {
	repo := newTestGraphRepository(t)
	ctx := context.Background()

	if err := repo.Save(ctx, newGraph(t, ownerTenant, singleNode("n1"))); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if _, err := repo.FindByEventPublicIDForUpdate(ctx, otherTenant, graphEvent); !errors.Is(err, repository.ErrGraphNotFound) {
		t.Errorf("FindByEventPublicIDForUpdate() with other tenant error = %v, want %v", err, repository.ErrGraphNotFound)
	}

	if _, err := repo.FindByEventPublicIDForUpdate(ctx, ownerTenant, "0123456789abcdef"); !errors.Is(err, repository.ErrGraphNotFound) {
		t.Errorf("FindByEventPublicIDForUpdate() with unknown event error = %v, want %v", err, repository.ErrGraphNotFound)
	}
}

func TestPostgresGraphRepositoryFindForUpdateSerializesWriters(t *testing.T) {
	repo := newTestGraphRepository(t)
	ctx := context.Background()

	if err := repo.Save(ctx, newGraph(t, ownerTenant, singleNode("n1"))); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	lock := func(ctx context.Context) error {
		_, err := repo.FindByEventPublicIDForUpdate(ctx, ownerTenant, graphEvent)

		return err
	}

	var contenderErr error

	err := repo.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := lock(txCtx); err != nil {
			return err
		}

		waitCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()

		contenderErr = repo.WithinTransaction(waitCtx, lock)

		return nil
	})
	if err != nil {
		t.Fatalf("WithinTransaction() error = %v", err)
	}

	if contenderErr == nil {
		t.Error("second FindByEventPublicIDForUpdate() while the first transaction holds the lock succeeded, want it to wait until the deadline")
	}

	if err := repo.WithinTransaction(ctx, lock); err != nil {
		t.Errorf("FindByEventPublicIDForUpdate() after the holder committed error = %v", err)
	}
}
