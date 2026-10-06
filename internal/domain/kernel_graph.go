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

type Point struct {
	ID             string
	Type           PointType
	IsBoundary     bool
	BoundaryActive bool
}

type Route struct {
	ID          string
	FromPointID string
	ToPointID   string
	Direction   DirectionAttribute
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
	Document       GraphDocument
}

type pointKind struct {
	pointType  PointType
	isBoundary bool
}

var pointKinds = map[NodeType]pointKind{
	NodeTypeGoal:             {pointType: PointTypeGoal, isBoundary: false},
	NodeTypeGoalTransitMixed: {pointType: PointTypeGoalTransitMixed, isBoundary: false},
	NodeTypeTransitOnly:      {pointType: PointTypeTransitOnly, isBoundary: false},
	NodeTypeBoundary:         {pointType: PointTypeTransitOnly, isBoundary: true},
}

var directionAttributes = map[EdgeDirection]DirectionAttribute{
	EdgeDirectionOneWay:   DirectionAttributeOneWay,
	EdgeDirectionBothWays: DirectionAttributeBothWays,
}

func (r PublishedRevision) KernelGraph() KernelGraph {
	points := make([]Point, 0, len(r.Document.Nodes))
	for _, node := range r.Document.Nodes {
		kind := pointKinds[node.Type]
		points = append(points, Point{
			ID:             node.ID,
			Type:           kind.pointType,
			IsBoundary:     kind.isBoundary,
			BoundaryActive: true,
		})
	}

	routes := make([]Route, 0, len(r.Document.Edges))
	for _, edge := range r.Document.Edges {
		routes = append(routes, Route{
			ID:          edge.ID,
			FromPointID: edge.SourceNodeID,
			ToPointID:   edge.TargetNodeID,
			Direction:   directionAttributes[edge.Direction],
		})
	}

	return KernelGraph{
		EventPublicID: r.EventPublicID,
		RevisionID:    r.RevisionID,
		Points:        points,
		Routes:        routes,
	}
}
