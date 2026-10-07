package db

import "github.com/pj-hoakari/tolo-graph-authoring/internal/domain"

type graphDraft struct {
	EventPublicID  string `db:"event_public_id"`
	TenantPublicID string `db:"tenant_public_id"`
	RevisionID     string `db:"revision_id"`
	storedDocument
}

func newGraphDraft(graph domain.Graph) (graphDraft, error) {
	document, err := newStoredDocument(graph.Draft())
	if err != nil {
		return graphDraft{}, err
	}

	return graphDraft{
		EventPublicID:  graph.EventPublicID(),
		TenantPublicID: graph.TenantPublicID(),
		RevisionID:     graph.DraftRevisionID(),
		storedDocument: document,
	}, nil
}

func (d graphDraft) graph(currentRevisionID string) (domain.Graph, error) {
	return domain.RestoreGraph(d.TenantPublicID, d.EventPublicID, d.graphDocument(), currentRevisionID)
}
