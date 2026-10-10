CREATE TABLE graph_layouts (
    event_public_id TEXT PRIMARY KEY,
    tenant_public_id TEXT NOT NULL,
    layout JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (event_public_id, tenant_public_id) REFERENCES graphs (event_public_id, tenant_public_id)
);
