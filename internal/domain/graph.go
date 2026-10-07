// Package domain contains the immutable models of the graph authoring
// context: the graph of an event with its draft and published
// revisions, and the shared kernel graph derived from a published revision.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrInvalidGraphDocument = errors.New("invalid graph document")

type NodeType int

const (
	NodeTypeUnspecified NodeType = iota
	NodeTypeGoal
	NodeTypeGoalTransitMixed
	NodeTypeTransitOnly
	NodeTypeBoundary
)

type EdgeDirection int

const (
	EdgeDirectionUnspecified EdgeDirection = iota
	EdgeDirectionOneWay
	EdgeDirectionBothWays
)

type Layout struct {
	X, Y          float64
	Width, Height *float64
}

type Node struct {
	ID      string
	Type    NodeType
	Labels  map[string]string
	GroupID string
	Layout  Layout
}

type Group struct {
	ID     string
	Labels map[string]string
	Layout Layout
}

type Edge struct {
	ID           string
	SourceNodeID string
	TargetNodeID string
	Direction    EdgeDirection
	Label        *string
}

type GraphDocument struct {
	Nodes  []Node
	Groups []Group
	Edges  []Edge
}

type Graph struct {
	tenantPublicID  string
	eventPublicID   string
	draft           GraphDocument
	draftRevisionID string
	revisionID      string
}

func NewGraph(tenantPublicID, eventPublicID string, draft GraphDocument) (Graph, error) {
	return Graph{
		tenantPublicID:  tenantPublicID,
		eventPublicID:   eventPublicID,
		draft:           GraphDocument{Nodes: nil, Groups: nil, Edges: nil},
		draftRevisionID: "",
		revisionID:      "",
	}.WithDraft(draft)
}

func RestoreGraph(tenantPublicID, eventPublicID string, draft GraphDocument, revisionID string) (Graph, error) {
	graph, err := NewGraph(tenantPublicID, eventPublicID, draft)
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
	encoded, err := json.Marshal(draft)
	if err != nil {
		return Graph{}, fmt.Errorf("%w: %w", ErrInvalidGraphDocument, err)
	}

	sum := sha256.Sum256(encoded)
	g.draft = draft
	g.draftRevisionID = hex.EncodeToString(sum[:8])

	return g, nil
}

func (g Graph) TenantPublicID() string  { return g.tenantPublicID }
func (g Graph) EventPublicID() string   { return g.eventPublicID }
func (g Graph) Draft() GraphDocument    { return g.draft }
func (g Graph) DraftRevisionID() string { return g.draftRevisionID }
func (g Graph) RevisionID() string      { return g.revisionID }
