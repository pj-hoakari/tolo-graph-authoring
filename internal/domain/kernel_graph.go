package domain

type PointType int

const (
	PointTypeUnspecified PointType = iota
	PointTypeGoal
	PointTypeGoalTransitMixed
	PointTypeTransitOnly
)

type DirectionAttribute int

const (
	DirectionAttributeUnspecified DirectionAttribute = iota
	DirectionAttributeOneWay
	DirectionAttributeBothWays
)

type BoundaryDirection int

const (
	BoundaryDirectionEntry        BoundaryDirection = 1
	BoundaryDirectionExit         BoundaryDirection = 2
	BoundaryDirectionEntryAndExit                   = BoundaryDirectionEntry | BoundaryDirectionExit
)

type PointBoundary struct {
	Direction BoundaryDirection `json:"direction"`
	Active    bool              `json:"active"`
}

type Point struct {
	ID       string         `json:"id"`
	Type     PointType      `json:"type"`
	Boundary *PointBoundary `json:"boundary,omitempty"`
}

type Route struct {
	ID          string             `json:"id"`
	FromPointID string             `json:"from"`
	ToPointID   string             `json:"to"`
	Direction   DirectionAttribute `json:"direction"`
}

type KernelGraph struct {
	EventPublicID string
	RevisionID    string
	Points        []Point
	Routes        []Route
}

type PublishedRevision struct {
	TenantPublicID string
	EventPublicID  string
	RevisionID     string
	Kernel         GraphKernel
}

func (r PublishedRevision) KernelGraph() KernelGraph {
	return KernelGraph{
		EventPublicID: r.EventPublicID,
		RevisionID:    r.RevisionID,
		Points:        r.Kernel.Points,
		Routes:        r.Kernel.Routes,
	}
}
