package db

import "github.com/pj-hoakari/tolo-graph-authoring/internal/domain"

type graphRevision struct {
	EventPublicID  string `db:"event_public_id"`
	TenantPublicID string `db:"tenant_public_id"`
	RevisionID     string `db:"revision_id"`
	storedDocument
}

func (r graphRevision) publishedRevision() domain.PublishedRevision {
	return domain.PublishedRevision{
		TenantPublicID: r.TenantPublicID,
		EventPublicID:  r.EventPublicID,
		RevisionID:     r.RevisionID,
		Document:       r.graphDocument(),
	}
}
