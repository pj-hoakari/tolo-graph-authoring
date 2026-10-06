package db

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

type draftColumns struct {
	Kernel []byte `db:"kernel"`
	Labels []byte `db:"labels"`
	Layout []byte `db:"layout"`
}

type graphKernel struct {
	Nodes  []kernelNode  `json:"nodes"`
	Groups []kernelGroup `json:"groups"`
	Edges  []kernelEdge  `json:"edges"`
}

type kernelNode struct {
	ID      string          `json:"id"`
	Type    domain.NodeType `json:"type"`
	GroupID string          `json:"group_id"`
}

type kernelGroup struct {
	ID string `json:"id"`
}

type kernelEdge struct {
	ID        string               `json:"id"`
	Source    string               `json:"source"`
	Target    string               `json:"target"`
	Direction domain.EdgeDirection `json:"direction"`
}

type graphLabels struct {
	Nodes  map[string]map[string]string `json:"nodes"`
	Groups map[string]map[string]string `json:"groups"`
	Edges  map[string]*string           `json:"edges"`
}

type graphLayout struct {
	Nodes  map[string]layoutColumn `json:"nodes"`
	Groups map[string]layoutColumn `json:"groups"`
}

type layoutColumn struct {
	X      float64  `json:"x"`
	Y      float64  `json:"y"`
	Width  *float64 `json:"width"`
	Height *float64 `json:"height"`
}

func encodeDraft(document domain.GraphDocument) (draftColumns, error) {
	kernel := graphKernel{
		Nodes:  emptyLike[kernelNode](document.Nodes),
		Groups: emptyLike[kernelGroup](document.Groups),
		Edges:  emptyLike[kernelEdge](document.Edges),
	}
	labels := graphLabels{
		Nodes:  make(map[string]map[string]string, len(document.Nodes)),
		Groups: make(map[string]map[string]string, len(document.Groups)),
		Edges:  make(map[string]*string, len(document.Edges)),
	}
	layout := graphLayout{
		Nodes:  make(map[string]layoutColumn, len(document.Nodes)),
		Groups: make(map[string]layoutColumn, len(document.Groups)),
	}

	for _, node := range document.Nodes {
		if _, ok := labels.Nodes[node.ID]; ok {
			return draftColumns{}, duplicateID("node", node.ID)
		}

		kernel.Nodes = append(kernel.Nodes, kernelNode{ID: node.ID, Type: node.Type, GroupID: node.GroupID})
		labels.Nodes[node.ID] = node.Labels
		layout.Nodes[node.ID] = layoutColumn(node.Layout)
	}

	for _, group := range document.Groups {
		if _, ok := labels.Groups[group.ID]; ok {
			return draftColumns{}, duplicateID("group", group.ID)
		}

		kernel.Groups = append(kernel.Groups, kernelGroup{ID: group.ID})
		labels.Groups[group.ID] = group.Labels
		layout.Groups[group.ID] = layoutColumn(group.Layout)
	}

	for _, edge := range document.Edges {
		if _, ok := labels.Edges[edge.ID]; ok {
			return draftColumns{}, duplicateID("edge", edge.ID)
		}

		kernel.Edges = append(kernel.Edges, kernelEdge{
			ID:        edge.ID,
			Source:    edge.SourceNodeID,
			Target:    edge.TargetNodeID,
			Direction: edge.Direction,
		})
		labels.Edges[edge.ID] = edge.Label
	}

	var (
		columns draftColumns
		errs    [3]error
	)

	columns.Kernel, errs[0] = json.Marshal(kernel)
	columns.Labels, errs[1] = json.Marshal(labels)
	columns.Layout, errs[2] = json.Marshal(layout)

	if err := errors.Join(errs[:]...); err != nil {
		return draftColumns{}, fmt.Errorf("%w: %w", domain.ErrInvalidGraphDocument, err)
	}

	return columns, nil
}

func decodeDraft(columns draftColumns) (domain.GraphDocument, error) {
	var (
		kernel graphKernel
		labels graphLabels
		layout graphLayout
	)

	if err := errors.Join(
		json.Unmarshal(columns.Kernel, &kernel),
		json.Unmarshal(columns.Labels, &labels),
		json.Unmarshal(columns.Layout, &layout),
	); err != nil {
		return domain.GraphDocument{}, fmt.Errorf("decode graph draft: %w", err)
	}

	document := domain.GraphDocument{
		Nodes:  emptyLike[domain.Node](kernel.Nodes),
		Groups: emptyLike[domain.Group](kernel.Groups),
		Edges:  emptyLike[domain.Edge](kernel.Edges),
	}

	for _, node := range kernel.Nodes {
		document.Nodes = append(document.Nodes, domain.Node{
			ID:      node.ID,
			Type:    node.Type,
			Labels:  labels.Nodes[node.ID],
			GroupID: node.GroupID,
			Layout:  domain.Layout(layout.Nodes[node.ID]),
		})
	}

	for _, group := range kernel.Groups {
		document.Groups = append(document.Groups, domain.Group{
			ID:     group.ID,
			Labels: labels.Groups[group.ID],
			Layout: domain.Layout(layout.Groups[group.ID]),
		})
	}

	for _, edge := range kernel.Edges {
		document.Edges = append(document.Edges, domain.Edge{
			ID:           edge.ID,
			SourceNodeID: edge.Source,
			TargetNodeID: edge.Target,
			Direction:    edge.Direction,
			Label:        labels.Edges[edge.ID],
		})
	}

	return document, nil
}

func emptyLike[U, T any](source []T) []U {
	if source == nil {
		return nil
	}

	return make([]U, 0, len(source))
}

func duplicateID(kind, id string) error {
	return fmt.Errorf("%w: duplicate %s ID %q", domain.ErrInvalidGraphDocument, kind, id)
}
