CREATE TABLE graph_revisions (
    event_public_id TEXT NOT NULL,
    tenant_public_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    kernel JSONB NOT NULL,
    labels JSONB NOT NULL,
    layout JSONB NOT NULL,
    last_published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_public_id, revision_id),
    FOREIGN KEY (event_public_id, tenant_public_id) REFERENCES graphs (event_public_id, tenant_public_id)
);

CREATE INDEX graph_revisions_current_idx ON graph_revisions (event_public_id, last_published_at DESC);
