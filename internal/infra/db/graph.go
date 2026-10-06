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

func (r *PostgresGraphRepository) FindByEventPublicIDForUpdate(
	ctx context.Context, tenantPublicID, eventPublicID string,
) (domain.VenueGraph, error) {
	var row struct {
		draftColumns

		TenantPublicID string `db:"tenant_public_id"`
	}

	err := sqlx.GetContext(ctx, r.executor(ctx), &row, `
		SELECT d.tenant_public_id, d.kernel, d.labels, d.layout
		FROM graph_drafts d
		JOIN graphs g ON g.event_public_id = d.event_public_id
		WHERE d.event_public_id = $1 AND d.tenant_public_id = $2
		FOR UPDATE OF g`,
		eventPublicID, tenantPublicID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.VenueGraph{}, repository.ErrGraphNotFound
	}

	if err != nil {
		return domain.VenueGraph{}, fmt.Errorf("find graph draft: %w", err)
	}

	document, err := decodeDraft(row.draftColumns)
	if err != nil {
		return domain.VenueGraph{}, err
	}

	return domain.NewVenueGraph(row.TenantPublicID, eventPublicID, document)
}

func (r *PostgresGraphRepository) Save(ctx context.Context, graph domain.VenueGraph) error {
	columns, err := encodeDraft(graph.Draft())
	if err != nil {
		return err
	}

	return r.WithinTransaction(ctx, func(ctx context.Context) error {
		owner, err := r.executor(ctx).ExecContext(ctx, `
			INSERT INTO graphs (event_public_id, tenant_public_id)
			VALUES ($1, $2)
			ON CONFLICT (event_public_id) DO UPDATE SET tenant_public_id = EXCLUDED.tenant_public_id
			WHERE graphs.tenant_public_id = EXCLUDED.tenant_public_id`,
			graph.EventPublicID(), graph.TenantPublicID())
		if err := requireRowWritten(owner, err, "save graph owner"); err != nil {
			return err
		}

		draft, err := r.executor(ctx).ExecContext(ctx, `
			INSERT INTO graph_drafts (event_public_id, tenant_public_id, revision_id, kernel, labels, layout)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (event_public_id) DO UPDATE SET
				revision_id = EXCLUDED.revision_id,
				kernel = EXCLUDED.kernel,
				labels = EXCLUDED.labels,
				layout = EXCLUDED.layout,
				updated_at = now()
			WHERE graph_drafts.tenant_public_id = EXCLUDED.tenant_public_id`,
			graph.EventPublicID(), graph.TenantPublicID(), graph.DraftRevisionID(),
			string(columns.Kernel), string(columns.Labels), string(columns.Layout))

		return requireRowWritten(draft, err, "save graph draft")
	})
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
