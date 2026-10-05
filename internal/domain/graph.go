package domain

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

func NewVenueGraph(tenantPublicID, eventPublicID string, draft GraphDocument, draftRevisionID string) VenueGraph {
	return VenueGraph{
		tenantPublicID:  tenantPublicID,
		eventPublicID:   eventPublicID,
		draft:           draft,
		draftRevisionID: draftRevisionID,
		revisionID:      "",
	}
}

func (g VenueGraph) WithDraft(draft GraphDocument, draftRevisionID string) VenueGraph {
	g.draft = draft
	g.draftRevisionID = draftRevisionID

	return g
}

func (g VenueGraph) TenantPublicID() string  { return g.tenantPublicID }
func (g VenueGraph) EventPublicID() string   { return g.eventPublicID }
func (g VenueGraph) Draft() GraphDocument    { return g.draft }
func (g VenueGraph) DraftRevisionID() string { return g.draftRevisionID }
func (g VenueGraph) RevisionID() string      { return g.revisionID }
