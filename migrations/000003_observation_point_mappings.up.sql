CREATE TABLE observation_point_mappings (
    event_public_id TEXT NOT NULL,
    tenant_public_id TEXT NOT NULL,
    observation_point_id TEXT NOT NULL,
    anchor_kind TEXT NOT NULL CHECK (anchor_kind IN ('point', 'route')),
    anchor_element_id TEXT NOT NULL,
    route_position DOUBLE PRECISION CHECK (route_position BETWEEN 0 AND 1),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_public_id, observation_point_id),
    FOREIGN KEY (event_public_id, tenant_public_id) REFERENCES graphs (event_public_id, tenant_public_id),
    CHECK (route_position IS NULL OR anchor_kind = 'route')
);
