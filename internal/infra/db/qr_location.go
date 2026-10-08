package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
)

type qrLocation struct {
	QrLocationID string `db:"qr_location_id"`
	Name         string `db:"name"`
	Kind         string `db:"kind"`
	storedAnchor
}

func (r *PostgresGraphRepository) FindPlacements(ctx context.Context, graph domain.Graph) (domain.Placements, error) {
	var mappings []observationPointMapping

	err := sqlx.SelectContext(ctx, r.executor(ctx), &mappings, `
		SELECT observation_point_id, anchor_kind, anchor_element_id, route_position
		FROM observation_point_mappings
		WHERE event_public_id = $1 AND tenant_public_id = $2
		ORDER BY observation_point_id`,
		graph.EventPublicID(), graph.TenantPublicID())
	if err != nil {
		return domain.Placements{}, fmt.Errorf("find observation point mappings: %w", err)
	}

	var locations []qrLocation

	err = sqlx.SelectContext(ctx, r.executor(ctx), &locations, `
		SELECT qr_location_id, name, kind, anchor_kind, anchor_element_id, route_position
		FROM qr_locations
		WHERE event_public_id = $1 AND tenant_public_id = $2
		ORDER BY qr_location_id`,
		graph.EventPublicID(), graph.TenantPublicID())
	if err != nil {
		return domain.Placements{}, fmt.Errorf("find QR locations: %w", err)
	}

	placements := domain.Placements{
		Mappings:    make([]domain.ObservationPointMapping, 0, len(mappings)),
		QrLocations: make([]domain.QrLocation, 0, len(locations)),
	}
	for _, row := range mappings {
		placements.Mappings = append(placements.Mappings, row.mapping())
	}

	for _, row := range locations {
		placements.QrLocations = append(placements.QrLocations, domain.QrLocation{
			ID:     row.QrLocationID,
			Name:   row.Name,
			Kind:   row.Kind,
			Anchor: row.graphAnchor(),
		})
	}

	return placements, nil
}

func (r *PostgresGraphRepository) AddQrLocation(ctx context.Context, graph domain.Graph, location domain.QrLocation) error {
	_, err := r.executor(ctx).ExecContext(ctx, `
		INSERT INTO qr_locations
			(event_public_id, tenant_public_id, qr_location_id, name, kind, anchor_kind, anchor_element_id, route_position)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		graph.EventPublicID(), graph.TenantPublicID(), location.ID, location.Name, location.Kind,
		anchorKindNames[location.Anchor.Kind], location.Anchor.ElementID, location.Anchor.RoutePosition)
	if err != nil {
		return fmt.Errorf("add QR location: %w", err)
	}

	return nil
}

func (r *PostgresGraphRepository) UpdateQrLocation(ctx context.Context, graph domain.Graph, location domain.QrLocation) error {
	result, err := r.executor(ctx).ExecContext(ctx, `
		UPDATE qr_locations SET
			name = $4,
			kind = $5,
			anchor_kind = $6,
			anchor_element_id = $7,
			route_position = $8,
			updated_at = now()
		WHERE event_public_id = $1 AND tenant_public_id = $2 AND qr_location_id = $3`,
		graph.EventPublicID(), graph.TenantPublicID(), location.ID, location.Name, location.Kind,
		anchorKindNames[location.Anchor.Kind], location.Anchor.ElementID, location.Anchor.RoutePosition)

	return requireQrLocationFound(result, err, "update QR location")
}

func (r *PostgresGraphRepository) RemoveQrLocation(ctx context.Context, graph domain.Graph, qrLocationID string) error {
	result, err := r.executor(ctx).ExecContext(ctx, `
		DELETE FROM qr_locations
		WHERE event_public_id = $1 AND tenant_public_id = $2 AND qr_location_id = $3`,
		graph.EventPublicID(), graph.TenantPublicID(), qrLocationID)

	return requireQrLocationFound(result, err, "remove QR location")
}

func requireQrLocationFound(result sql.Result, err error, operation string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	if affected == 0 {
		return repository.ErrQrLocationNotFound
	}

	return nil
}
