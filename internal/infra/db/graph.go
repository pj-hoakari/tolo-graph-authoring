package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/tenantctx"
)

type PostgresGraphRepository struct {
	db *sqlx.DB
}

func NewPostgresGraphRepository(db *sqlx.DB) *PostgresGraphRepository {
	return &PostgresGraphRepository{db: db}
}

func (r *PostgresGraphRepository) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return RunInTransaction(ctx, r.db, fn)
}

func (r *PostgresGraphRepository) executor(ctx context.Context) sqlx.ExtContext {
	return Executor(ctx, r.db)
}

func (r *PostgresGraphRepository) FindByEventPublicID(ctx context.Context, tenantPublicID, eventPublicID string) (domain.Graph, error) {
	return r.findByEventPublicID(ctx, tenantPublicID, eventPublicID, "")
}

func (r *PostgresGraphRepository) FindByEventPublicIDForUpdate(
	ctx context.Context, tenantPublicID, eventPublicID string,
) (domain.Graph, error) {
	return r.findByEventPublicID(ctx, tenantPublicID, eventPublicID, "FOR UPDATE OF g")
}

func (r *PostgresGraphRepository) FindDraftKernelForUpdate(
	ctx context.Context, tenantPublicID, eventPublicID string,
) (domain.DraftKernel, error) {
	var row struct {
		EventPublicID     string                         `db:"event_public_id"`
		TenantPublicID    string                         `db:"tenant_public_id"`
		Kernel            jsonColumn[domain.GraphKernel] `db:"kernel"`
		DocumentHash      string                         `db:"doc_hash"`
		CurrentRevisionID string                         `db:"current_revision_id"`
	}

	err := sqlx.GetContext(ctx, r.executor(ctx), &row, `
		SELECT d.event_public_id, d.tenant_public_id, d.kernel, d.doc_hash,
			`+currentRevisionIDColumn+`
		FROM graph_drafts d
		JOIN graphs g ON g.event_public_id = d.event_public_id
		WHERE d.event_public_id = $1 AND d.tenant_public_id = $2
		FOR UPDATE OF g`,
		eventPublicID, tenantPublicID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DraftKernel{}, repository.ErrGraphNotFound
	}

	if err != nil {
		return domain.DraftKernel{}, fmt.Errorf("find graph draft kernel: %w", err)
	}

	return domain.DraftKernel{
		GraphRef:     domain.GraphRef{TenantPublicID: row.TenantPublicID, EventPublicID: row.EventPublicID},
		Kernel:       row.Kernel.value,
		DocumentHash: row.DocumentHash,
		RevisionID:   row.CurrentRevisionID,
	}, nil
}

func (r *PostgresGraphRepository) FindDraftRevisionForUpdate(
	ctx context.Context, tenantPublicID, eventPublicID string,
) (domain.DraftRevision, error) {
	var row struct {
		EventPublicID  string `db:"event_public_id"`
		TenantPublicID string `db:"tenant_public_id"`
		RevisionID     string `db:"revision_id"`
	}

	err := sqlx.GetContext(ctx, r.executor(ctx), &row, `
		SELECT d.event_public_id, d.tenant_public_id, d.revision_id
		FROM graph_drafts d
		JOIN graphs g ON g.event_public_id = d.event_public_id
		WHERE d.event_public_id = $1 AND d.tenant_public_id = $2
		FOR UPDATE OF g`,
		eventPublicID, tenantPublicID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DraftRevision{}, repository.ErrGraphNotFound
	}

	if err != nil {
		return domain.DraftRevision{}, fmt.Errorf("find graph draft revision: %w", err)
	}

	return domain.DraftRevision{
		GraphRef:   domain.GraphRef{TenantPublicID: row.TenantPublicID, EventPublicID: row.EventPublicID},
		RevisionID: row.RevisionID,
	}, nil
}

func (r *PostgresGraphRepository) findByEventPublicID(
	ctx context.Context, tenantPublicID, eventPublicID, lock string,
) (domain.Graph, error) {
	var row struct {
		graphDraft

		CurrentRevisionID string `db:"current_revision_id"`
	}

	err := sqlx.GetContext(ctx, r.executor(ctx), &row, `
		SELECT d.event_public_id, d.tenant_public_id, d.revision_id, d.kernel, d.labels, l.layout,
			`+currentRevisionIDColumn+`
		FROM graph_drafts d
		JOIN graphs g ON g.event_public_id = d.event_public_id
		JOIN graph_layouts l ON l.event_public_id = d.event_public_id
		WHERE d.event_public_id = $1 AND d.tenant_public_id = $2
		`+lock,
		eventPublicID, tenantPublicID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Graph{}, repository.ErrGraphNotFound
	}

	if err != nil {
		return domain.Graph{}, fmt.Errorf("find graph draft: %w", err)
	}

	placements, err := r.FindPlacements(ctx, domain.GraphRef{TenantPublicID: row.TenantPublicID, EventPublicID: row.EventPublicID})
	if err != nil {
		return domain.Graph{}, err
	}

	return row.graph(placements, row.CurrentRevisionID)
}

const currentRevisionIDColumn = `COALESCE((
				SELECT r.revision_id FROM graph_revisions r
				WHERE r.event_public_id = d.event_public_id
				ORDER BY r.last_published_at DESC
				LIMIT 1
			), '') AS current_revision_id`

func (r *PostgresGraphRepository) Save(ctx context.Context, graph domain.Graph) error {
	draft := newGraphDraft(graph)

	return r.WithinTransaction(ctx, func(ctx context.Context) error {
		owner, err := r.executor(ctx).ExecContext(ctx, `
			INSERT INTO graphs (event_public_id, tenant_public_id)
			VALUES ($1, $2)
			ON CONFLICT (event_public_id) DO UPDATE SET tenant_public_id = EXCLUDED.tenant_public_id
			WHERE graphs.tenant_public_id = EXCLUDED.tenant_public_id`,
			draft.EventPublicID, draft.TenantPublicID)
		if err := requireRowWritten(owner, err, "save graph owner"); err != nil {
			return err
		}

		saved, err := r.executor(ctx).ExecContext(ctx, `
			INSERT INTO graph_drafts (event_public_id, tenant_public_id, revision_id, doc_hash, kernel, labels)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (event_public_id) DO UPDATE SET
				revision_id = EXCLUDED.revision_id,
				doc_hash = EXCLUDED.doc_hash,
				kernel = EXCLUDED.kernel,
				labels = EXCLUDED.labels,
				updated_at = now()
			WHERE graph_drafts.tenant_public_id = EXCLUDED.tenant_public_id`,
			draft.EventPublicID, draft.TenantPublicID, draft.RevisionID, draft.DocumentHash,
			draft.Kernel, draft.Labels)
		if err := requireRowWritten(saved, err, "save graph draft"); err != nil {
			return err
		}

		laidOut, err := r.executor(ctx).ExecContext(ctx, `
			INSERT INTO graph_layouts (event_public_id, tenant_public_id, layout)
			VALUES ($1, $2, $3)
			ON CONFLICT (event_public_id) DO UPDATE SET
				layout = EXCLUDED.layout,
				updated_at = now()
			WHERE graph_layouts.tenant_public_id = EXCLUDED.tenant_public_id`,
			draft.EventPublicID, draft.TenantPublicID, draft.Layout)

		return requireRowWritten(laidOut, err, "save graph layout")
	})
}

func (r *PostgresGraphRepository) SaveDraftRevisionID(ctx context.Context, graph domain.GraphRef, revisionID string) error {
	result, err := r.executor(ctx).ExecContext(ctx, `
		UPDATE graph_drafts SET revision_id = $3, updated_at = now()
		WHERE event_public_id = $1 AND tenant_public_id = $2`,
		graph.EventPublicID, graph.TenantPublicID, revisionID)

	return requireRowWritten(result, err, "save graph draft revision ID")
}

func (r *PostgresGraphRepository) Publish(ctx context.Context, graph domain.GraphRef, placements domain.Placements) error {
	result, err := r.executor(ctx).ExecContext(ctx, `
		INSERT INTO graph_revisions (event_public_id, tenant_public_id, revision_id, kernel, labels, placements)
		SELECT event_public_id, tenant_public_id, revision_id, kernel, labels, $3
		FROM graph_drafts
		WHERE event_public_id = $1 AND tenant_public_id = $2
		ON CONFLICT (event_public_id, revision_id) DO UPDATE SET last_published_at = EXCLUDED.last_published_at`,
		graph.EventPublicID, graph.TenantPublicID, jsonColumn[domain.Placements]{value: placements})
	if err != nil {
		return fmt.Errorf("publish graph revision: %w", err)
	}

	published, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("publish graph revision: %w", err)
	}

	if published == 0 {
		return repository.ErrGraphNotFound
	}

	return nil
}

func (r *PostgresGraphRepository) FindCurrentRevision(ctx context.Context, eventPublicID string) (domain.PublishedRevision, error) {
	var revision graphRevision

	err := sqlx.GetContext(ctx, Executor(ctx, r.db), &revision, `
		SELECT event_public_id, tenant_public_id, revision_id, kernel
		FROM graph_revisions
		WHERE event_public_id = $1
		ORDER BY last_published_at DESC
		LIMIT 1`,
		eventPublicID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishedRevision{}, repository.ErrGraphNotFound
	}

	if err != nil {
		return domain.PublishedRevision{}, fmt.Errorf("find current graph revision: %w", err)
	}

	return revision.publishedRevision(), nil
}

func (r *PostgresGraphRepository) FindRevision(ctx context.Context, eventPublicID, revisionID string) (domain.PublishedRevision, error) {
	var revision graphRevision

	err := sqlx.GetContext(ctx, r.executor(ctx), &revision, `
		SELECT event_public_id, tenant_public_id, revision_id, kernel, placements
		FROM graph_revisions
		WHERE event_public_id = $1 AND revision_id = $2`,
		eventPublicID, revisionID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishedRevision{}, repository.ErrRevisionNotFound
	}

	if err != nil {
		return domain.PublishedRevision{}, fmt.Errorf("find graph revision: %w", err)
	}

	return revision.publishedRevision(), nil
}

func requireRowWritten(result sql.Result, err error, operation string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	written, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	if written == 0 {
		return fmt.Errorf("%s: %w", operation, tenantctx.ErrMismatch)
	}

	return nil
}
