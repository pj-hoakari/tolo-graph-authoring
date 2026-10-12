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

type DraftKernel struct {
	GraphRef

	Kernel GraphKernel
}

func (k DraftKernel) VerifyMapping(mapping ObservationPointMapping) error {
	if mapping.ObservationPointID == "" {
		return ErrInvalidObservationPointMapping
	}

	return k.verifyAnchor(mapping.Anchor, ErrInvalidObservationPointMapping)
}

func (k DraftKernel) VerifyQrLocation(location QrLocation) error {
	if location.Name == "" || location.Kind == "" {
		return ErrInvalidQrLocation
	}

	return k.verifyAnchor(location.Anchor, ErrInvalidQrLocation)
}

func (k DraftKernel) verifyAnchor(anchor GraphAnchor, invalid error) error {
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

	if !k.Kernel.contains(anchor) {
		return ErrAnchorTargetNotFound
	}

	return nil
}

func (k GraphKernel) contains(anchor GraphAnchor) bool {
	switch anchor.Kind {
	case AnchorKindPoint:
		return slices.ContainsFunc(k.Points, func(point Point) bool { return point.ID == anchor.ElementID })
	case AnchorKindRoute:
		return slices.ContainsFunc(k.Routes, func(route Route) bool { return route.ID == anchor.ElementID })
	case AnchorKindUnspecified:
	}

	return false
}

func (g Graph) VerifyPlacements(placements Placements) error {
	for _, mapping := range placements.Mappings {
		if !g.draft.Kernel.contains(mapping.Anchor) {
			return fmt.Errorf("%w: observation point %q", ErrAnchorTargetNotFound, mapping.ObservationPointID)
		}
	}

	for _, location := range placements.QrLocations {
		if !g.draft.Kernel.contains(location.Anchor) {
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
