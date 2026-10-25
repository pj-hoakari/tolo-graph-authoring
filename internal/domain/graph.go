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
	"strings"
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
	var violations GraphViolations

	nodes := make(map[string]Node, len(d.Nodes))
	checkedNodes := make([]Node, 0, len(d.Nodes))

	for _, node := range d.Nodes {
		if node.ID == "" {
			violations.add(ViolationKindEmptyNodeID, ElementKindNode, "", nil, "node has an empty ID")

			continue
		}

		if _, ok := nodes[node.ID]; ok {
			violations.add(ViolationKindDuplicateNodeID, ElementKindNode, node.ID, nil, "duplicate node ID %q", node.ID)

			continue
		}

		if node.Type == NodeTypeUnspecified {
			violations.add(ViolationKindNodeWithoutType, ElementKindNode, node.ID, nil, "node %q has no type", node.ID)
		}

		if !node.Layout.finite() {
			violations.add(ViolationKindNonFiniteNodeLayout, ElementKindNode, node.ID, nil, "node %q has a non-finite layout", node.ID)
		}

		nodes[node.ID] = node
		checkedNodes = append(checkedNodes, node)
	}

	groups := make(map[string]Group, len(d.Groups))
	checkedGroups := make([]Group, 0, len(d.Groups))

	for _, group := range d.Groups {
		if group.ID == "" {
			violations.add(ViolationKindEmptyGroupID, ElementKindGroup, "", nil, "group has an empty ID")

			continue
		}

		if _, ok := groups[group.ID]; ok {
			violations.add(ViolationKindDuplicateGroupID, ElementKindGroup, group.ID, nil, "duplicate group ID %q", group.ID)

			continue
		}

		if !group.Layout.finite() || !finite(group.MinWidth) || !finite(group.MinHeight) {
			violations.add(ViolationKindNonFiniteGroupLayout, ElementKindGroup, group.ID, nil, "group %q has a non-finite layout", group.ID)
		}

		groups[group.ID] = group
		checkedGroups = append(checkedGroups, group)
	}

	populated := make(map[string]bool, len(groups))

	for _, node := range checkedNodes {
		if node.GroupID == "" {
			continue
		}

		populated[node.GroupID] = true

		if node.Type == NodeTypeExternal {
			violations.add(ViolationKindExternalNodeInGroup, ElementKindNode, node.ID, []string{node.GroupID},
				"external node %q belongs to group %q", node.ID, node.GroupID)
		}

		if _, ok := groups[node.GroupID]; !ok {
			violations.add(ViolationKindMissingGroup, ElementKindNode, node.ID, []string{node.GroupID},
				"node %q refers to missing group %q", node.ID, node.GroupID)
		}
	}

	for _, group := range checkedGroups {
		if group.ParentGroupID == "" {
			continue
		}

		populated[group.ParentGroupID] = true

		if _, ok := groups[group.ParentGroupID]; !ok {
			violations.add(ViolationKindMissingParentGroup, ElementKindGroup, group.ID, []string{group.ParentGroupID},
				"group %q refers to missing parent group %q", group.ID, group.ParentGroupID)

			continue
		}

		for depth, parentID := 0, group.ParentGroupID; parentID != ""; depth, parentID = depth+1, groups[parentID].ParentGroupID {
			if depth == len(groups) {
				violations.add(ViolationKindGroupCycle, ElementKindGroup, group.ID, nil, "group %q is nested in a cycle", group.ID)

				break
			}
		}
	}

	linked := make(map[string]bool, len(nodes))
	routed := make(map[string]bool, len(nodes))

	edges := make(map[string]bool, len(d.Edges))
	for _, edge := range d.Edges {
		if edge.ID == "" {
			violations.add(ViolationKindEmptyEdgeID, ElementKindEdge, "", nil, "edge has an empty ID")

			continue
		}

		if edges[edge.ID] {
			violations.add(ViolationKindDuplicateEdgeID, ElementKindEdge, edge.ID, nil, "duplicate edge ID %q", edge.ID)

			continue
		}

		edges[edge.ID] = true

		if edge.Direction == EdgeDirectionUnspecified {
			violations.add(ViolationKindEdgeWithoutDirection, ElementKindEdge, edge.ID, nil, "edge %q has no direction", edge.ID)
		}

		source, sourceFound := nodes[edge.SourceNodeID]
		target, targetFound := nodes[edge.TargetNodeID]

		if !sourceFound || !targetFound {
			var missing []string
			if !sourceFound {
				missing = append(missing, edge.SourceNodeID)
			}

			if !targetFound {
				missing = append(missing, edge.TargetNodeID)
			}

			violations.add(ViolationKindMissingNode, ElementKindEdge, edge.ID, missing, "edge %q refers to a missing node", edge.ID)

			continue
		}

		if source.Type == NodeTypeExternal && target.Type == NodeTypeExternal {
			violations.add(ViolationKindEdgeBetweenExternalNodes, ElementKindEdge, edge.ID, nil, "edge %q connects two external nodes", edge.ID)
		}

		if edge.SourceNodeID == edge.TargetNodeID {
			violations.add(ViolationKindSelfLoopEdge, ElementKindEdge, edge.ID, []string{edge.SourceNodeID},
				"edge %q connects node %q to itself", edge.ID, edge.SourceNodeID)
		}

		if source.Type == NodeTypeExternal || target.Type == NodeTypeExternal {
			linked[edge.SourceNodeID], linked[edge.TargetNodeID] = true, true

			continue
		}

		routed[edge.SourceNodeID], routed[edge.TargetNodeID] = true, true
	}

	for _, node := range checkedNodes {
		if node.Type == NodeTypeExternal && !linked[node.ID] {
			violations.add(ViolationKindExternalNodeWithoutEdge, ElementKindNode, node.ID, nil, "external node %q has no edge", node.ID)
		}

		if node.Type != NodeTypeExternal && !routed[node.ID] {
			violations.add(ViolationKindNodeWithoutRoute, ElementKindNode, node.ID, nil, "point %q has no route", node.ID)
		}
	}

	for _, group := range checkedGroups {
		if !populated[group.ID] {
			violations.add(ViolationKindGroupWithoutMembers, ElementKindGroup, group.ID, nil, "group %q has no members", group.ID)
		}
	}

	if len(violations) > 0 {
		return violations
	}

	return nil
}

func (l Layout) finite() bool {
	return finite(&l.X) && finite(&l.Y) && finite(l.Width) && finite(l.Height)
}

func finite(value *float64) bool {
	return value == nil || !math.IsNaN(*value) && !math.IsInf(*value, 0)
}

type ViolationKind int

const (
	ViolationKindUnspecified ViolationKind = iota
	ViolationKindEmptyNodeID
	ViolationKindDuplicateNodeID
	ViolationKindNodeWithoutType
	ViolationKindNonFiniteNodeLayout
	ViolationKindEmptyGroupID
	ViolationKindDuplicateGroupID
	ViolationKindNonFiniteGroupLayout
	ViolationKindExternalNodeInGroup
	ViolationKindMissingGroup
	ViolationKindMissingParentGroup
	ViolationKindGroupCycle
	ViolationKindEmptyEdgeID
	ViolationKindDuplicateEdgeID
	ViolationKindEdgeWithoutDirection
	ViolationKindMissingNode
	ViolationKindEdgeBetweenExternalNodes
	ViolationKindSelfLoopEdge
	ViolationKindExternalNodeWithoutEdge
	ViolationKindNodeWithoutRoute
	ViolationKindGroupWithoutMembers
)

type ElementKind int

const (
	ElementKindUnspecified ElementKind = iota
	ElementKindNode
	ElementKindGroup
	ElementKindEdge
)

type GraphViolation struct {
	Kind        ViolationKind
	ElementKind ElementKind
	ElementID   string
	RelatedIDs  []string
	message     string
}

type GraphViolations []GraphViolation

func (v *GraphViolations) add(kind ViolationKind, element ElementKind, id string, related []string, format string, args ...any) {
	*v = append(*v, GraphViolation{Kind: kind, ElementKind: element, ElementID: id, RelatedIDs: related, message: fmt.Sprintf(format, args...)})
}

func (v GraphViolations) Error() string {
	messages := make([]string, len(v))
	for i, violation := range v {
		messages[i] = violation.message
	}

	return fmt.Sprintf("%v: %s", ErrInvalidGraphDocument, strings.Join(messages, "; "))
}

func (v GraphViolations) Unwrap() error { return ErrInvalidGraphDocument }

func (v GraphViolation) Message() string { return v.message }

type GraphRef struct {
	TenantPublicID string
	EventPublicID  string
}

type DraftRevision struct {
	GraphRef

	RevisionID string
}

type Graph struct {
	tenantPublicID  string
	eventPublicID   string
	draft           GraphParts
	documentHash    string
	placements      Placements
	draftRevisionID string
	revisionID      string
}

type GraphMeta struct {
	EventPublicID   string
	RevisionID      string
	DraftRevisionID string
}

func NewGraph(tenantPublicID, eventPublicID string, draft GraphDocument) (Graph, error) {
	var graph Graph

	graph.tenantPublicID, graph.eventPublicID = tenantPublicID, eventPublicID

	return graph.WithDraft(draft)
}

func RestoreGraph(tenantPublicID, eventPublicID string, draft GraphParts, placements Placements, revisionID string) (Graph, error) {
	var graph Graph

	graph.tenantPublicID, graph.eventPublicID, graph.placements = tenantPublicID, eventPublicID, placements

	graph, err := graph.WithDraft(draft.Document())
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

	if err := g.placements.verifyKept(parts); err != nil {
		return Graph{}, err
	}

	documentHash, err := parts.documentHash()
	if err != nil {
		return Graph{}, err
	}

	revisionID, err := RevisionID(documentHash, g.placements)
	if err != nil {
		return Graph{}, err
	}

	g.draft = parts
	g.documentHash = documentHash
	g.draftRevisionID = revisionID

	return g, nil
}

func (g Graph) TenantPublicID() string  { return g.tenantPublicID }
func (g Graph) EventPublicID() string   { return g.eventPublicID }
func (g Graph) Draft() GraphDocument    { return g.draft.Document() }
func (g Graph) DraftParts() GraphParts  { return g.draft }
func (g Graph) DocumentHash() string    { return g.documentHash }
func (g Graph) Placements() Placements  { return g.placements }
func (g Graph) DraftRevisionID() string { return g.draftRevisionID }
func (g Graph) RevisionID() string      { return g.revisionID }

func (g Graph) Ref() GraphRef {
	return GraphRef{TenantPublicID: g.tenantPublicID, EventPublicID: g.eventPublicID}
}

func (g Graph) Meta() GraphMeta {
	return GraphMeta{EventPublicID: g.eventPublicID, RevisionID: g.revisionID, DraftRevisionID: g.draftRevisionID}
}

func (p GraphParts) documentHash() (string, error) {
	return hashJSON(struct {
		Kernel GraphKernel `json:"kernel"`
		Labels GraphLabels `json:"labels"`
	}{p.Kernel, p.Labels})
}

func RevisionID(documentHash string, placements Placements) (string, error) {
	return hashJSON(struct {
		Document   string     `json:"document"`
		Placements Placements `json:"placements"`
	}{documentHash, placements.canonical()})
}

func hashJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:8]), nil
}
