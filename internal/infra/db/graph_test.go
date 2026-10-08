package db

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
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

	if _, err := testDB.Exec(`TRUNCATE qr_locations, observation_point_mappings, graph_revisions, graph_drafts, graphs`); err != nil {
		t.Fatalf("truncate graph tables: %v", err)
	}

	return NewPostgresGraphRepository(testDB)
}

func newGraph(t *testing.T, tenantPublicID string, document domain.GraphDocument) domain.Graph {
	t.Helper()

	graph, err := domain.NewGraph(tenantPublicID, graphEvent, document)
	if err != nil {
		t.Fatalf("NewGraph() error = %v", err)
	}

	return graph
}

func singleNode(id string) domain.GraphDocument {
	return domain.GraphDocument{Nodes: []domain.Node{{ID: id, Type: domain.NodeTypeGoal}}}
}

func assertDraft(t *testing.T, repo *PostgresGraphRepository, want domain.Graph) {
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

	if got.RevisionID() != want.RevisionID() {
		t.Errorf("loaded RevisionID() = %q, want %q", got.RevisionID(), want.RevisionID())
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

type callerTenantEvents struct{}

func (callerTenantEvents) FindEvent(ctx context.Context, eventPublicID string) (domain.Event, error) {
	tenantPublicID, _ := tenantctx.TenantPublicIDFromContext(ctx)

	return domain.NewEvent(eventPublicID, tenantPublicID, false), nil
}

func TestSaveGraphOfAnotherTenantLeavesOwnerDraft(t *testing.T) {
	repo := newTestGraphRepository(t)
	service := application.NewGraphService(repo, repo, callerTenantEvents{})

	save := func(tenantPublicID string, document domain.GraphDocument) (domain.Graph, error) {
		ctx := internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
			TokenUse:       internaljwt.TokenUseEventAccess,
			TenantPublicID: tenantPublicID,
			EventPublicID:  graphEvent,
		})

		return service.SaveGraph(ctx, application.SaveGraphInput{EventPublicID: graphEvent, Document: &document})
	}

	owned, err := save(ownerTenant, singleNode("owner"))
	if err != nil {
		t.Fatalf("owner SaveGraph() error = %v", err)
	}

	if _, err := save(otherTenant, singleNode("intruder")); !errors.Is(err, tenantctx.ErrMismatch) {
		t.Fatalf("other tenant SaveGraph() error = %v, want %v", err, tenantctx.ErrMismatch)
	}

	assertDraft(t, repo, owned)

	var graphs int
	if err := testDB.Get(&graphs, `SELECT COUNT(*) FROM graphs WHERE tenant_public_id = $1`, otherTenant); err != nil {
		t.Fatalf("count graphs of the other tenant: %v", err)
	}

	if graphs != 0 {
		t.Errorf("graphs owned by the other tenant = %d, want 0", graphs)
	}
}

func saveGraph(t *testing.T, repo *PostgresGraphRepository, document domain.GraphDocument) domain.Graph {
	t.Helper()

	graph := newGraph(t, ownerTenant, document)
	if err := repo.Save(context.Background(), graph); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	return graph
}

func publishGraph(t *testing.T, repo *PostgresGraphRepository, graph domain.Graph) domain.Graph {
	t.Helper()

	if err := repo.Publish(context.Background(), graph); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	return graph.Published()
}

func countRevisions(t *testing.T) int {
	t.Helper()

	var revisions int
	if err := testDB.Get(&revisions, `SELECT COUNT(*) FROM graph_revisions`); err != nil {
		t.Fatalf("count revisions: %v", err)
	}

	return revisions
}

func TestPostgresGraphRepositoryPublishCopiesDraft(t *testing.T) {
	repo := newTestGraphRepository(t)
	published := publishGraph(t, repo, saveGraph(t, repo, richDocument()))

	var copies int
	if err := testDB.Get(&copies, `
		SELECT COUNT(*) FROM graph_revisions r
		JOIN graph_drafts d USING (event_public_id, tenant_public_id, revision_id)
		WHERE r.kernel = d.kernel AND r.labels = d.labels AND r.layout = d.layout`); err != nil {
		t.Fatalf("count revisions equal to the draft: %v", err)
	}

	if copies != 1 || countRevisions(t) != 1 {
		t.Errorf("revisions equal to the draft = %d of %d, want 1 of 1", copies, countRevisions(t))
	}

	assertDraft(t, repo, published)
}

func TestPostgresGraphRepositoryRepublishMakesOlderRevisionCurrent(t *testing.T) {
	repo := newTestGraphRepository(t)

	first := publishGraph(t, repo, saveGraph(t, repo, singleNode("a")))
	publishGraph(t, repo, saveGraph(t, repo, singleNode("b")))
	saveGraph(t, repo, singleNode("a"))
	publishGraph(t, repo, first)

	assertDraft(t, repo, first)

	if got := countRevisions(t); got != 2 {
		t.Errorf("revision rows = %d, want 2", got)
	}
}

func TestPostgresGraphRepositoryPublishWithoutOwnedDraftWritesNothing(t *testing.T) {
	repo := newTestGraphRepository(t)
	ctx := context.Background()

	if err := repo.Publish(ctx, newGraph(t, ownerTenant, singleNode("n1"))); !errors.Is(err, repository.ErrGraphNotFound) {
		t.Errorf("Publish() without a draft error = %v, want %v", err, repository.ErrGraphNotFound)
	}

	saveGraph(t, repo, singleNode("n1"))

	if err := repo.Publish(ctx, newGraph(t, otherTenant, singleNode("n1"))); !errors.Is(err, repository.ErrGraphNotFound) {
		t.Errorf("Publish() by another tenant error = %v, want %v", err, repository.ErrGraphNotFound)
	}

	if got := countRevisions(t); got != 0 {
		t.Errorf("revision rows = %d, want 0", got)
	}
}

func TestPostgresGraphRepositoryFindReportsEditingAgainstCurrentRevision(t *testing.T) {
	repo := newTestGraphRepository(t)
	ctx := context.Background()

	published := publishGraph(t, repo, saveGraph(t, repo, singleNode("a")))
	saveGraph(t, repo, singleNode("b"))

	editing, err := repo.FindByEventPublicIDForUpdate(ctx, ownerTenant, graphEvent)
	if err != nil {
		t.Fatalf("FindByEventPublicIDForUpdate() error = %v", err)
	}

	if editing.RevisionID() != published.RevisionID() || editing.DraftRevisionID() == editing.RevisionID() {
		t.Errorf("after editing RevisionID() = %q, DraftRevisionID() = %q, want revision %q and a different draft",
			editing.RevisionID(), editing.DraftRevisionID(), published.RevisionID())
	}

	saveGraph(t, repo, singleNode("a"))

	assertDraft(t, repo, published)
}

func TestPostgresGraphRepositoryFindCurrentRevisionOfEvent(t *testing.T) {
	repo := newTestGraphRepository(t)
	ctx := context.Background()

	if _, err := repo.FindCurrentRevision(ctx, graphEvent); !errors.Is(err, repository.ErrGraphNotFound) {
		t.Errorf("FindCurrentRevision() before publishing error = %v, want %v", err, repository.ErrGraphNotFound)
	}

	publishGraph(t, repo, saveGraph(t, repo, singleNode("old")))
	current := publishGraph(t, repo, saveGraph(t, repo, richDocument()))

	otherEvent, err := domain.NewGraph(otherTenant, "0123456789abcdef", singleNode("elsewhere"))
	if err != nil {
		t.Fatalf("NewGraph() error = %v", err)
	}

	if err := repo.Save(ctx, otherEvent); err != nil {
		t.Fatalf("Save() of another event error = %v", err)
	}

	publishGraph(t, repo, otherEvent)

	got, err := repo.FindCurrentRevision(ctx, graphEvent)
	if err != nil {
		t.Fatalf("FindCurrentRevision() error = %v", err)
	}

	want := domain.PublishedRevision{
		TenantPublicID: ownerTenant,
		EventPublicID:  graphEvent,
		RevisionID:     current.RevisionID(),
		Document:       richDocument(),
	}

	if got.TenantPublicID != want.TenantPublicID || got.RevisionID != want.RevisionID {
		t.Errorf("FindCurrentRevision() owner and revision = %s/%s, want %s/%s", got.TenantPublicID, got.RevisionID, want.TenantPublicID, want.RevisionID)
	}

	if !reflect.DeepEqual(got.KernelGraph(), want.KernelGraph()) {
		t.Errorf("FindCurrentRevision() kernel graph = %+v, want %+v", got.KernelGraph(), want.KernelGraph())
	}
}
