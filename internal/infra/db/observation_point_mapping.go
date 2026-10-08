package db

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

var anchorKindNames = map[domain.AnchorKind]string{
	domain.AnchorKindPoint: "point",
	domain.AnchorKindRoute: "route",
}

var anchorKinds = map[string]domain.AnchorKind{
	"point": domain.AnchorKindPoint,
	"route": domain.AnchorKindRoute,
}

type storedAnchor struct {
	AnchorKind      string   `db:"anchor_kind"`
	AnchorElementID string   `db:"anchor_element_id"`
	RoutePosition   *float64 `db:"route_position"`
}

func (a storedAnchor) graphAnchor() domain.GraphAnchor {
	return domain.GraphAnchor{Kind: anchorKinds[a.AnchorKind], ElementID: a.AnchorElementID, RoutePosition: a.RoutePosition}
}

type observationPointMapping struct {
	ObservationPointID string `db:"observation_point_id"`
	storedAnchor
}

func (m observationPointMapping) mapping() domain.ObservationPointMapping {
	return domain.ObservationPointMapping{ObservationPointID: m.ObservationPointID, Anchor: m.graphAnchor()}
}

func (r *PostgresGraphRepository) SaveObservationPointMapping(
	ctx context.Context, graph domain.Graph, mapping domain.ObservationPointMapping,
) error {
	result, err := r.executor(ctx).ExecContext(ctx, `
		INSERT INTO observation_point_mappings
			(event_public_id, tenant_public_id, observation_point_id, anchor_kind, anchor_element_id, route_position)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_public_id, observation_point_id) DO UPDATE SET
			anchor_kind = EXCLUDED.anchor_kind,
			anchor_element_id = EXCLUDED.anchor_element_id,
			route_position = EXCLUDED.route_position,
			updated_at = now()
		WHERE observation_point_mappings.tenant_public_id = EXCLUDED.tenant_public_id`,
		graph.EventPublicID(), graph.TenantPublicID(), mapping.ObservationPointID,
		anchorKindNames[mapping.Anchor.Kind], mapping.Anchor.ElementID, mapping.Anchor.RoutePosition)

	return requireRowWritten(result, err, "save observation point mapping")
}

func (r *PostgresGraphRepository) FindObservationPointMappings(
	ctx context.Context, eventPublicID string,
) ([]domain.ObservationPointMapping, error) {
	var rows []observationPointMapping

	err := sqlx.SelectContext(ctx, r.executor(ctx), &rows, `
		SELECT observation_point_id, anchor_kind, anchor_element_id, route_position
		FROM observation_point_mappings
		WHERE event_public_id = $1
		ORDER BY observation_point_id`,
		eventPublicID)
	if err != nil {
		return nil, fmt.Errorf("find observation point mappings: %w", err)
	}

	mappings := make([]domain.ObservationPointMapping, 0, len(rows))
	for _, row := range rows {
		mappings = append(mappings, row.mapping())
	}

	return mappings, nil
}
