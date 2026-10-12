ALTER TABLE graph_drafts ADD COLUMN doc_hash TEXT;
UPDATE graph_drafts SET doc_hash = revision_id;
ALTER TABLE graph_drafts ALTER COLUMN doc_hash SET NOT NULL;

ALTER TABLE graph_revisions ADD COLUMN placements JSONB NOT NULL DEFAULT '{"mappings": [], "qr_locations": []}';
ALTER TABLE graph_revisions ALTER COLUMN placements DROP DEFAULT;
