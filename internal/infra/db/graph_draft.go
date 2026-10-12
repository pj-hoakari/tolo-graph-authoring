package db

import "github.com/pj-hoakari/tolo-graph-authoring/internal/domain"

type graphDraft struct {
	EventPublicID  string                         `db:"event_public_id"`
	TenantPublicID string                         `db:"tenant_public_id"`
	RevisionID     string                         `db:"revision_id"`
	DocumentHash   string                         `db:"doc_hash"`
	Kernel         jsonColumn[domain.GraphKernel] `db:"kernel"`
	Labels         jsonColumn[domain.GraphLabels] `db:"labels"`
	Layout         jsonColumn[domain.GraphLayout] `db:"layout"`
}

func newGraphDraft(graph domain.Graph) graphDraft {
	parts := graph.DraftParts()

	return graphDraft{
		EventPublicID:  graph.EventPublicID(),
		TenantPublicID: graph.TenantPublicID(),
		RevisionID:     graph.DraftRevisionID(),
		DocumentHash:   graph.DocumentHash(),
		Kernel:         jsonColumn[domain.GraphKernel]{value: parts.Kernel},
		Labels:         jsonColumn[domain.GraphLabels]{value: parts.Labels},
		Layout:         jsonColumn[domain.GraphLayout]{value: parts.Layout},
	}
}

func (d graphDraft) graph(placements domain.Placements, currentRevisionID string) (domain.Graph, error) {
	parts := domain.GraphParts{Kernel: d.Kernel.value, Labels: d.Labels.value, Layout: d.Layout.value}

	return domain.RestoreGraph(d.TenantPublicID, d.EventPublicID, parts, placements, currentRevisionID)
}
