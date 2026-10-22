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
	Labels         GraphLabels
	Placements     Placements
}

func (r PublishedRevision) KernelGraph() KernelGraph {
	return KernelGraph{
		EventPublicID: r.EventPublicID,
		RevisionID:    r.RevisionID,
		Points:        r.Kernel.Points,
		Routes:        r.Kernel.Routes,
	}
}

type RouteEndpoints struct {
	FromPointID string
	ToPointID   string
}

type DisplayNames struct {
	RevisionID     string
	Points         map[string]map[string]string
	Routes         map[string]string
	RouteEndpoints map[string]RouteEndpoints
}

func (r PublishedRevision) DisplayNames() DisplayNames {
	names := DisplayNames{
		RevisionID:     r.RevisionID,
		Points:         make(map[string]map[string]string),
		Routes:         make(map[string]string),
		RouteEndpoints: make(map[string]RouteEndpoints),
	}

	for _, point := range r.Kernel.Points {
		texts := make(map[string]string)

		for language, text := range r.Labels.Points[point.ID] {
			if text != "" {
				texts[language] = text
			}
		}

		if len(texts) > 0 {
			names.Points[point.ID] = texts
		}
	}

	for _, route := range r.Kernel.Routes {
		if label := r.Labels.Routes[route.ID]; label != nil && *label != "" {
			names.Routes[route.ID] = *label
		}

		names.RouteEndpoints[route.ID] = RouteEndpoints{FromPointID: route.FromPointID, ToPointID: route.ToPointID}
	}

	return names
}
