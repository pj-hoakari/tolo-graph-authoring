CREATE TABLE graphs (
    event_public_id TEXT PRIMARY KEY,
    tenant_public_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_public_id, tenant_public_id)
);
