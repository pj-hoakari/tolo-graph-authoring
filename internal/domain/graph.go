// Package domain contains the immutable models of the graph authoring
// context: the graph of an event with its draft and published
// revisions, and the shared kernel graph derived from a published revision.
package domain

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
)

var ErrInvalidGraphDocument = errors.New("invalid graph document")

type NodeType int

const (
	NodeTypeUnspecified NodeType = iota
	NodeTypeGoal
	NodeTypeGoalTransitMixed
	NodeTypeTransitOnly
	NodeTypeExternal
)

type EdgeDirection int

const (
	EdgeDirectionUnspecified EdgeDirection = iota
	EdgeDirectionOneWay
	EdgeDirectionBothWays
)

type Layout struct {
	X      float64  `json:"x"`
	Y      float64  `json:"y"`
	Width  *float64 `json:"width"`
	Height *float64 `json:"height"`
}

type Node struct {
	ID      string
	Type    NodeType
	Labels  map[string]string
	GroupID string
	Layout  Layout
}

type Group struct {
	ID            string
	Labels        map[string]string
	ParentGroupID string
	MinWidth      *float64
	MinHeight     *float64
	Layout        Layout
}

type Edge struct {
	ID           string        `json:"id"`
	SourceNodeID string        `json:"source"`
	TargetNodeID string        `json:"target"`
	Direction    EdgeDirection `json:"direction"`
	Label        *string       `json:"label"`
}

type GraphDocument struct {
	Nodes  []Node
	Groups []Group
	Edges  []Edge
}

func (d GraphDocument) canonical() GraphDocument {
	return GraphDocument{
		Nodes:  sortedByID(d.Nodes, func(node Node) string { return node.ID }),
		Groups: sortedByID(d.Groups, func(group Group) string { return group.ID }),
		Edges:  sortedByID(d.Edges, func(edge Edge) string { return edge.ID }),
	}
}

func sortedByID[T any](elements []T, id func(T) string) []T {
	sorted := make([]T, len(elements))
	copy(sorted, elements)
	slices.SortStableFunc(sorted, func(a, b T) int { return cmp.Compare(id(a), id(b)) })

	return sorted
}

func (d GraphDocument) validate() error {
	nodes := make(map[string]Node, len(d.Nodes))
	for _, node := range d.Nodes {
		if _, ok := nodes[node.ID]; ok {
			return invalidDocument("duplicate node ID %q", node.ID)
		}

		if node.Type == NodeTypeUnspecified {
			return invalidDocument("node %q has no type", node.ID)
		}

		if !node.Layout.finite() {
			return invalidDocument("node %q has a non-finite layout", node.ID)
		}

		nodes[node.ID] = node
	}

	groups := make(map[string]Group, len(d.Groups))
	for _, group := range d.Groups {
		if _, ok := groups[group.ID]; ok {
			return invalidDocument("duplicate group ID %q", group.ID)
		}

		if !group.Layout.finite() || !finite(group.MinWidth) || !finite(group.MinHeight) {
			return invalidDocument("group %q has a non-finite layout", group.ID)
		}

		groups[group.ID] = group
	}

	for _, node := range d.Nodes {
		if node.GroupID == "" {
			continue
		}

		if node.Type == NodeTypeExternal {
			return invalidDocument("external node %q belongs to group %q", node.ID, node.GroupID)
		}

		if _, ok := groups[node.GroupID]; !ok {
			return invalidDocument("node %q refers to missing group %q", node.ID, node.GroupID)
		}
	}

	for _, group := range d.Groups {
		for depth, parentID := 0, group.ParentGroupID; parentID != ""; depth, parentID = depth+1, groups[parentID].ParentGroupID {
			if _, ok := groups[parentID]; !ok {
				return invalidDocument("group %q refers to missing parent group %q", group.ID, parentID)
			}

			if depth == len(groups) {
				return invalidDocument("group %q is nested in a cycle", group.ID)
			}
		}
	}

	edges := make(map[string]bool, len(d.Edges))
	for _, edge := range d.Edges {
		if edges[edge.ID] {
			return invalidDocument("duplicate edge ID %q", edge.ID)
		}

		edges[edge.ID] = true

		if edge.Direction == EdgeDirectionUnspecified {
			return invalidDocument("edge %q has no direction", edge.ID)
		}

		source, sourceFound := nodes[edge.SourceNodeID]
		target, targetFound := nodes[edge.TargetNodeID]

		if !sourceFound || !targetFound {
			return invalidDocument("edge %q refers to a missing node", edge.ID)
		}

		if source.Type == NodeTypeExternal && target.Type == NodeTypeExternal {
			return invalidDocument("edge %q connects two external nodes", edge.ID)
		}
	}

	return nil
}

func (l Layout) finite() bool {
	return finite(&l.X) && finite(&l.Y) && finite(l.Width) && finite(l.Height)
}

func finite(value *float64) bool {
	return value == nil || !math.IsNaN(*value) && !math.IsInf(*value, 0)
}

func invalidDocument(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidGraphDocument, fmt.Sprintf(format, args...))
}

type Graph struct {
	tenantPublicID  string
	eventPublicID   string
	draft           GraphParts
	draftRevisionID string
	revisionID      string
}

func NewGraph(tenantPublicID, eventPublicID string, draft GraphDocument) (Graph, error) {
	var graph Graph

	graph.tenantPublicID, graph.eventPublicID = tenantPublicID, eventPublicID

	return graph.WithDraft(draft)
}

func RestoreGraph(tenantPublicID, eventPublicID string, draft GraphParts, revisionID string) (Graph, error) {
	graph, err := NewGraph(tenantPublicID, eventPublicID, draft.Document())
	if err != nil {
		return Graph{}, err
	}

	graph.revisionID = revisionID

	return graph, nil
}

func (g Graph) Published() Graph {
	g.revisionID = g.draftRevisionID

	return g
}

func (g Graph) WithDraft(draft GraphDocument) (Graph, error) {
	if err := draft.validate(); err != nil {
		return Graph{}, err
	}

	parts := draft.Parts()

	revisionID, err := parts.revisionID()
	if err != nil {
		return Graph{}, err
	}

	g.draft = parts
	g.draftRevisionID = revisionID

	return g, nil
}

func (g Graph) TenantPublicID() string  { return g.tenantPublicID }
func (g Graph) EventPublicID() string   { return g.eventPublicID }
func (g Graph) Draft() GraphDocument    { return g.draft.Document() }
func (g Graph) DraftParts() GraphParts  { return g.draft }
func (g Graph) DraftRevisionID() string { return g.draftRevisionID }
func (g Graph) RevisionID() string      { return g.revisionID }

func (p GraphParts) revisionID() (string, error) {
	encoded, err := json.Marshal(struct {
		Kernel GraphKernel `json:"kernel"`
		Labels GraphLabels `json:"labels"`
	}{p.Kernel, p.Labels})
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidGraphDocument, err)
	}

	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:8]), nil
}
