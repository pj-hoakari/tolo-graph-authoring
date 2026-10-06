// Package domain contains the immutable models of the graph authoring
// context: the venue graph of an event with its draft and published
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

type VenueGraph struct {
	tenantPublicID  string
	eventPublicID   string
	draft           GraphDocument
	draftRevisionID string
	revisionID      string
}

func NewVenueGraph(tenantPublicID, eventPublicID string, draft GraphDocument) (VenueGraph, error) {
	return VenueGraph{
		tenantPublicID:  tenantPublicID,
		eventPublicID:   eventPublicID,
		draft:           GraphDocument{Nodes: nil, Groups: nil, Edges: nil},
		draftRevisionID: "",
		revisionID:      "",
	}.WithDraft(draft)
}

func RestoreVenueGraph(tenantPublicID, eventPublicID string, draft GraphDocument, revisionID string) (VenueGraph, error) {
	graph, err := NewVenueGraph(tenantPublicID, eventPublicID, draft)
	if err != nil {
		return VenueGraph{}, err
	}

	graph.revisionID = revisionID

	return graph, nil
}

func (g VenueGraph) Published() VenueGraph {
	g.revisionID = g.draftRevisionID

	return g
}

func (g VenueGraph) WithDraft(draft GraphDocument) (VenueGraph, error) {
	encoded, err := json.Marshal(draft)
	if err != nil {
		return VenueGraph{}, fmt.Errorf("%w: %w", ErrInvalidGraphDocument, err)
	}

	sum := sha256.Sum256(encoded)
	g.draft = draft
	g.draftRevisionID = hex.EncodeToString(sum[:8])

	return g, nil
}

func (g VenueGraph) TenantPublicID() string  { return g.tenantPublicID }
func (g VenueGraph) EventPublicID() string   { return g.eventPublicID }
func (g VenueGraph) Draft() GraphDocument    { return g.draft }
func (g VenueGraph) DraftRevisionID() string { return g.draftRevisionID }
func (g VenueGraph) RevisionID() string      { return g.revisionID }
