package domain

import (
	"errors"
	"slices"
)

var (
	ErrInvalidObservationPointMapping = errors.New("invalid observation point mapping")
	ErrAnchorTargetNotFound           = errors.New("anchor target is not in the graph")
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

type ObservationPointMapping struct {
	ObservationPointID string
	Anchor             GraphAnchor
}

func (g Graph) VerifyMapping(mapping ObservationPointMapping) error {
	anchor := mapping.Anchor
	if mapping.ObservationPointID == "" || anchor.ElementID == "" {
		return ErrInvalidObservationPointMapping
	}

	var found bool

	switch anchor.Kind {
	case AnchorKindPoint:
		if anchor.RoutePosition != nil {
			return ErrInvalidObservationPointMapping
		}

		found = slices.ContainsFunc(g.draft.Nodes, func(node Node) bool { return node.ID == anchor.ElementID })
	case AnchorKindRoute:
		if position := anchor.RoutePosition; position != nil && !(*position >= 0 && *position <= 1) {
			return ErrInvalidObservationPointMapping
		}

		found = slices.ContainsFunc(g.draft.Edges, func(edge Edge) bool { return edge.ID == anchor.ElementID })
	case AnchorKindUnspecified:
		return ErrInvalidObservationPointMapping
	}

	if !found {
		return ErrAnchorTargetNotFound
	}

	return nil
}
