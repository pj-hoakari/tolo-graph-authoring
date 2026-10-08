package domain

import (
	"errors"
	"fmt"
	"slices"
)

var (
	ErrInvalidObservationPointMapping = errors.New("invalid observation point mapping")
	ErrInvalidQrLocation              = errors.New("invalid QR location")
	ErrAnchorTargetNotFound           = errors.New("anchor target is not in the graph")
	ErrPlacementConflict              = errors.New("camera mapping and QR location cannot share a graph element")
)

type AnchorKind int

const (
	AnchorKindUnspecified AnchorKind = iota
	AnchorKindPoint
	AnchorKindRoute
)

type GraphAnchor struct {
	Kind          AnchorKind
	ElementID     string
	RoutePosition *float64
}

func (a GraphAnchor) sameElement(other GraphAnchor) bool {
	return a.Kind == other.Kind && a.ElementID == other.ElementID
}

type ObservationPointMapping struct {
	ObservationPointID string
	Anchor             GraphAnchor
}

type QrLocation struct {
	ID     string
	Name   string
	Kind   string
	Anchor GraphAnchor
}

type Placements struct {
	Mappings    []ObservationPointMapping
	QrLocations []QrLocation
}

func (g Graph) VerifyMapping(mapping ObservationPointMapping) error {
	if mapping.ObservationPointID == "" {
		return ErrInvalidObservationPointMapping
	}

	return g.verifyAnchor(mapping.Anchor, ErrInvalidObservationPointMapping)
}

func (g Graph) VerifyQrLocation(location QrLocation) error {
	if location.Name == "" || location.Kind == "" {
		return ErrInvalidQrLocation
	}

	return g.verifyAnchor(location.Anchor, ErrInvalidQrLocation)
}

func (g Graph) verifyAnchor(anchor GraphAnchor, invalid error) error {
	if anchor.ElementID == "" {
		return invalid
	}

	switch anchor.Kind {
	case AnchorKindPoint:
		if anchor.RoutePosition != nil {
			return invalid
		}
	case AnchorKindRoute:
		if position := anchor.RoutePosition; position != nil && !(*position >= 0 && *position <= 1) {
			return invalid
		}
	case AnchorKindUnspecified:
		return invalid
	}

	if !g.contains(anchor) {
		return ErrAnchorTargetNotFound
	}

	return nil
}

func (g Graph) contains(anchor GraphAnchor) bool {
	switch anchor.Kind {
	case AnchorKindPoint:
		return slices.ContainsFunc(g.draft.Nodes, func(node Node) bool { return node.ID == anchor.ElementID })
	case AnchorKindRoute:
		return slices.ContainsFunc(g.draft.Edges, func(edge Edge) bool { return edge.ID == anchor.ElementID })
	case AnchorKindUnspecified:
	}

	return false
}

func (g Graph) VerifyPlacements(placements Placements) error {
	for _, mapping := range placements.Mappings {
		if !g.contains(mapping.Anchor) {
			return fmt.Errorf("%w: observation point %q", ErrAnchorTargetNotFound, mapping.ObservationPointID)
		}
	}

	for _, location := range placements.QrLocations {
		if !g.contains(location.Anchor) {
			return fmt.Errorf("%w: QR location %q", ErrAnchorTargetNotFound, location.ID)
		}
	}

	return nil
}

func (p Placements) VerifyMappable(anchor GraphAnchor) error {
	if slices.ContainsFunc(p.QrLocations, func(location QrLocation) bool { return location.Anchor.sameElement(anchor) }) {
		return ErrPlacementConflict
	}

	return nil
}

func (p Placements) VerifyQrPlaceable(anchor GraphAnchor) error {
	if slices.ContainsFunc(p.Mappings, func(mapping ObservationPointMapping) bool { return mapping.Anchor.sameElement(anchor) }) {
		return ErrPlacementConflict
	}

	return nil
}
