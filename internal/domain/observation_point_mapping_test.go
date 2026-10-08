package domain_test

import (
	"errors"
	"math"
	"testing"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

func TestVerifyMappingAcceptsOnlyElementsOfTheDraft(t *testing.T) {
	t.Parallel()

	graph, err := domain.NewGraph("a1b2c3d4e5f60718", "fedcba9876543210", domain.GraphDocument{
		Nodes: []domain.Node{{ID: "gate", Type: domain.NodeTypeBoundary}, {ID: "hall", Type: domain.NodeTypeGoal}},
		Edges: []domain.Edge{{ID: "e1", SourceNodeID: "gate", TargetNodeID: "hall", Direction: domain.EdgeDirectionOneWay}},
	})
	if err != nil {
		t.Fatalf("NewGraph() error = %v", err)
	}

	position := func(v float64) *float64 { return &v }

	for _, tc := range []struct {
		name               string
		observationPointID string
		anchor             domain.GraphAnchor
		want               error
	}{
		{"point", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "gate"}, nil},
		{"route without position", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindRoute, ElementID: "e1"}, nil},
		{"route at its end", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindRoute, ElementID: "e1", RoutePosition: position(1)}, nil},
		{"missing observation point ID", "", domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "gate"}, domain.ErrInvalidObservationPointMapping},
		{"missing anchor", "cam-1", domain.GraphAnchor{}, domain.ErrInvalidObservationPointMapping},
		{"point with position", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "gate", RoutePosition: position(0.5)}, domain.ErrInvalidObservationPointMapping},
		{"route position above 1", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindRoute, ElementID: "e1", RoutePosition: position(1.5)}, domain.ErrInvalidObservationPointMapping},
		{"route position NaN", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindRoute, ElementID: "e1", RoutePosition: position(math.NaN())}, domain.ErrInvalidObservationPointMapping},
		{"unknown point", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "nowhere"}, domain.ErrAnchorTargetNotFound},
		{"route ID as point", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "e1"}, domain.ErrAnchorTargetNotFound},
		{"point ID as route", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindRoute, ElementID: "gate"}, domain.ErrAnchorTargetNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mapping := domain.ObservationPointMapping{ObservationPointID: tc.observationPointID, Anchor: tc.anchor}
			if err := graph.VerifyMapping(mapping); !errors.Is(err, tc.want) {
				t.Errorf("VerifyMapping() error = %v, want %v", err, tc.want)
			}
		})
	}
}
