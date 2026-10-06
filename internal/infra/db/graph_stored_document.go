package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

type storedDocument struct {
	Kernel jsonColumn[graphKernel] `db:"kernel"`
	Labels jsonColumn[graphLabels] `db:"labels"`
	Layout jsonColumn[graphLayout] `db:"layout"`
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
	Nodes  map[string]storedLayout `json:"nodes"`
	Groups map[string]storedLayout `json:"groups"`
}

type storedLayout struct {
	X      float64  `json:"x"`
	Y      float64  `json:"y"`
	Width  *float64 `json:"width"`
	Height *float64 `json:"height"`
}

type jsonColumn[T any] struct {
	value T
}

func (c jsonColumn[T]) Value() (driver.Value, error) {
	encoded, err := json.Marshal(c.value)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidGraphDocument, err)
	}

	return string(encoded), nil
}

func (c *jsonColumn[T]) Scan(src any) error {
	switch src := src.(type) {
	case []byte:
		return json.Unmarshal(src, &c.value)
	case string:
		return json.Unmarshal([]byte(src), &c.value)
	default:
		return fmt.Errorf("scan JSON column: unsupported type %T", src)
	}
}

func newStoredDocument(document domain.GraphDocument) (storedDocument, error) {
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
		Nodes:  make(map[string]storedLayout, len(document.Nodes)),
		Groups: make(map[string]storedLayout, len(document.Groups)),
	}

	for _, node := range document.Nodes {
		if _, ok := labels.Nodes[node.ID]; ok {
			return storedDocument{}, duplicateID("node", node.ID)
		}

		kernel.Nodes = append(kernel.Nodes, kernelNode{ID: node.ID, Type: node.Type, GroupID: node.GroupID})
		labels.Nodes[node.ID] = node.Labels
		layout.Nodes[node.ID] = storedLayout(node.Layout)
	}

	for _, group := range document.Groups {
		if _, ok := labels.Groups[group.ID]; ok {
			return storedDocument{}, duplicateID("group", group.ID)
		}

		kernel.Groups = append(kernel.Groups, kernelGroup{ID: group.ID})
		labels.Groups[group.ID] = group.Labels
		layout.Groups[group.ID] = storedLayout(group.Layout)
	}

	for _, edge := range document.Edges {
		if _, ok := labels.Edges[edge.ID]; ok {
			return storedDocument{}, duplicateID("edge", edge.ID)
		}

		kernel.Edges = append(kernel.Edges, kernelEdge{
			ID:        edge.ID,
			Source:    edge.SourceNodeID,
			Target:    edge.TargetNodeID,
			Direction: edge.Direction,
		})
		labels.Edges[edge.ID] = edge.Label
	}

	return storedDocument{
		Kernel: jsonColumn[graphKernel]{value: kernel},
		Labels: jsonColumn[graphLabels]{value: labels},
		Layout: jsonColumn[graphLayout]{value: layout},
	}, nil
}

func (d storedDocument) graphDocument() domain.GraphDocument {
	kernel, labels, layout := d.Kernel.value, d.Labels.value, d.Layout.value
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

	return document
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
