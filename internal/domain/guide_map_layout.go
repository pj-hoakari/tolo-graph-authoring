package domain

import (
	"errors"
	"fmt"
	"maps"
	"math"
)

var (
	ErrInvalidGuideMapLayout        = errors.New("invalid guide map layout")
	ErrGuideMapLayoutTargetNotFound = errors.New("guide map layout target is not in the graph")
)

type GuideMapPlaceShape int

const (
	GuideMapPlaceShapeUnspecified GuideMapPlaceShape = iota
	GuideMapPlaceShapePin
	GuideMapPlaceShapeRect
	GuideMapPlaceShapeCircle
)

type GuideMapConnectorType int

const (
	GuideMapConnectorTypeUnspecified GuideMapConnectorType = iota
	GuideMapConnectorTypeStairs
	GuideMapConnectorTypeEscalator
	GuideMapConnectorTypeElevator
)

type GuideMapConnectorDirection int

const (
	GuideMapConnectorDirectionUnspecified GuideMapConnectorDirection = iota
	GuideMapConnectorDirectionBoth
	GuideMapConnectorDirectionUp
	GuideMapConnectorDirectionDown
)

type GuideMapLayout struct {
	Groups        []GuideMapGroup  `json:"groups"`
	Objects       []GuideMapObject `json:"objects"`
	North         float64          `json:"north"`
	Scale         float64          `json:"scale"`
	StartObjectID string           `json:"start_object_id"`
}

type GuideMapGroup struct {
	GroupID string `json:"group_id"`
}

type GuideMapObject struct {
	ID          string            `json:"id"`
	GroupID     string            `json:"group_id"`
	Title       map[string]string `json:"title"`
	Description map[string]string `json:"description"`
	Color       string            `json:"color"`
	Closed      bool              `json:"closed"`
	PointID     string            `json:"point_id"`
	RouteID     string            `json:"route_id"`
	Place       *GuideMapPlace    `json:"place,omitempty"`
	Line        *GuideMapLine     `json:"line,omitempty"`
	Area        *GuideMapArea     `json:"area,omitempty"`
	Text        *GuideMapText     `json:"text,omitempty"`
}

type GuideMapPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type GuideMapPlace struct {
	Center     GuideMapPosition   `json:"center"`
	Shape      GuideMapPlaceShape `json:"shape"`
	Width      float64            `json:"width"`
	Height     float64            `json:"height"`
	Pin        bool               `json:"pin"`
	Connector  *GuideMapConnector `json:"connector,omitempty"`
	MarkWidth  *float64           `json:"mark_width,omitempty"`
	MarkHeight *float64           `json:"mark_height,omitempty"`
}

type GuideMapConnector struct {
	Type      GuideMapConnectorType      `json:"type"`
	Link      string                     `json:"link"`
	Direction GuideMapConnectorDirection `json:"direction"`
}

type GuideMapLine struct {
	Points []GuideMapPosition `json:"points"`
	Width  float64            `json:"width"`
	Dash   bool               `json:"dash"`
}

type GuideMapArea struct {
	Points []GuideMapPosition `json:"points"`
}

type GuideMapText struct {
	Position GuideMapPosition `json:"position"`
	Size     float64          `json:"size"`
}

type GuideMapLabels struct {
	Points map[string]map[string]string
	Groups map[string]map[string]string
}

func (l GuideMapLayout) Validate() error {
	if err := l.validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidGuideMapLayout, err)
	}

	return nil
}

func (l GuideMapLayout) validate() error {
	if !finiteValues(l.North, l.Scale) {
		return errors.New("north and scale must be finite")
	}

	groups := make(map[string]bool, len(l.Groups))
	for _, group := range l.Groups {
		if group.GroupID == "" || groups[group.GroupID] {
			return fmt.Errorf("group %q is empty or duplicated", group.GroupID)
		}

		groups[group.GroupID] = true
	}

	places := make(map[string]bool, len(l.Objects))
	objects := make(map[string]bool, len(l.Objects))

	for _, object := range l.Objects {
		if object.ID == "" || objects[object.ID] {
			return fmt.Errorf("object %q is empty or duplicated", object.ID)
		}

		objects[object.ID] = true

		if !groups[object.GroupID] {
			return fmt.Errorf("object %q is on group %q, which is not in the layout", object.ID, object.GroupID)
		}

		if err := object.validateKind(); err != nil {
			return fmt.Errorf("object %q: %w", object.ID, err)
		}

		places[object.ID] = object.Place != nil
	}

	if l.StartObjectID != "" && !places[l.StartObjectID] {
		return fmt.Errorf("start object %q is not a place", l.StartObjectID)
	}

	return nil
}

func (o GuideMapObject) validateKind() error {
	kinds := 0

	for _, set := range []bool{o.Place != nil, o.Line != nil, o.Area != nil, o.Text != nil} {
		if set {
			kinds++
		}
	}

	if kinds != 1 {
		return errors.New("exactly one kind must be set")
	}

	switch {
	case o.Place != nil:
		return o.Place.validate()
	case o.Line != nil:
		if !finitePositions(o.Line.Points) || !finiteValues(o.Line.Width) {
			return errors.New("line must be finite")
		}
	case o.Area != nil:
		if !finitePositions(o.Area.Points) {
			return errors.New("area must be finite")
		}
	case o.Text != nil:
		if !finitePositions([]GuideMapPosition{o.Text.Position}) || !finiteValues(o.Text.Size) {
			return errors.New("text must be finite")
		}
	}

	return nil
}

func (p GuideMapPlace) validate() error {
	if p.Shape == GuideMapPlaceShapeUnspecified {
		return errors.New("place has no shape")
	}

	if !finitePositions([]GuideMapPosition{p.Center}) || !finiteValues(p.Width, p.Height) || !finite(p.MarkWidth) || !finite(p.MarkHeight) {
		return errors.New("place must be finite")
	}

	if connector := p.Connector; connector != nil &&
		(connector.Type == GuideMapConnectorTypeUnspecified ||
			connector.Direction == GuideMapConnectorDirectionUnspecified ||
			connector.Link == "") {
		return errors.New("connector needs a type, a direction and a link")
	}

	return nil
}

func finiteValues(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

func finitePositions(positions []GuideMapPosition) bool {
	for _, position := range positions {
		if !finiteValues(position.X, position.Y) {
			return false
		}
	}

	return true
}

func (l GuideMapLayout) verifyTargets(kernel GraphKernel, labels GraphLabels) error {
	for _, group := range l.Groups {
		if _, ok := labels.Groups[group.GroupID]; !ok {
			return fmt.Errorf("%w: group %q", ErrGuideMapLayoutTargetNotFound, group.GroupID)
		}
	}

	for _, object := range l.Objects {
		if object.PointID != "" && !kernel.contains(GraphAnchor{Kind: AnchorKindPoint, ElementID: object.PointID, RoutePosition: nil}) {
			return fmt.Errorf("%w: point %q of object %q", ErrGuideMapLayoutTargetNotFound, object.PointID, object.ID)
		}

		if object.RouteID != "" && !kernel.contains(GraphAnchor{Kind: AnchorKindRoute, ElementID: object.RouteID, RoutePosition: nil}) {
			return fmt.Errorf("%w: route %q of object %q", ErrGuideMapLayoutTargetNotFound, object.RouteID, object.ID)
		}
	}

	return nil
}

func (l GuideMapLayout) Labels(labels GraphLabels) GuideMapLabels {
	points := make(map[string]map[string]string)

	for _, object := range l.Objects {
		if text, ok := labels.Points[object.PointID]; ok && object.PointID != "" {
			points[object.PointID] = maps.Clone(text)
		}
	}

	groups := make(map[string]map[string]string, len(l.Groups))
	for _, group := range l.Groups {
		if text, ok := labels.Groups[group.GroupID]; ok {
			groups[group.GroupID] = maps.Clone(text)
		}
	}

	return GuideMapLabels{Points: points, Groups: groups}
}
