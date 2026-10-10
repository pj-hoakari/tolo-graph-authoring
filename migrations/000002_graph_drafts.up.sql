CREATE TABLE graph_drafts (
    event_public_id TEXT PRIMARY KEY,
    tenant_public_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    kernel JSONB NOT NULL,
    labels JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (event_public_id, tenant_public_id) REFERENCES graphs (event_public_id, tenant_public_id)
);
