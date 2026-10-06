package domain_test

import (
	"reflect"
	"testing"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

func kernelGraphOf(document domain.GraphDocument) domain.KernelGraph {
	return domain.PublishedRevision{
		TenantPublicID: "a1b2c3d4e5f60718",
		EventPublicID:  "fedcba9876543210",
		RevisionID:     "0123456789abcdef",
		Document:       document,
	}.KernelGraph()
}

func TestKernelGraphDerivesPointFromEachNodeType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		nodeType   domain.NodeType
		pointType  domain.PointType
		isBoundary bool
	}{
		{domain.NodeTypeUnspecified, domain.PointTypeUnspecified, false},
		{domain.NodeTypeGoal, domain.PointTypeGoal, false},
		{domain.NodeTypeGoalTransitMixed, domain.PointTypeGoalTransitMixed, false},
		{domain.NodeTypeTransitOnly, domain.PointTypeTransitOnly, false},
		{domain.NodeTypeBoundary, domain.PointTypeTransitOnly, true},
	} {
		got := kernelGraphOf(domain.GraphDocument{Nodes: []domain.Node{{ID: "n1", Type: tc.nodeType}}}).Points
		want := []domain.Point{{ID: "n1", Type: tc.pointType, IsBoundary: tc.isBoundary, BoundaryActive: true}}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("points of node type %v = %+v, want %+v", tc.nodeType, got, want)
		}
	}
}

func TestKernelGraphDerivesRouteFromEachEdgeDirection(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		direction domain.EdgeDirection
		want      domain.DirectionAttribute
	}{
		{domain.EdgeDirectionUnspecified, domain.DirectionAttributeUnspecified},
		{domain.EdgeDirectionOneWay, domain.DirectionAttributeOneWay},
		{domain.EdgeDirectionBothWays, domain.DirectionAttributeBothWays},
	} {
		got := kernelGraphOf(domain.GraphDocument{Edges: []domain.Edge{{ID: "e1", SourceNodeID: "a", TargetNodeID: "b", Direction: tc.direction}}}).Routes
		want := []domain.Route{{ID: "e1", FromPointID: "a", ToPointID: "b", Direction: tc.want}}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("routes of edge direction %v = %+v, want %+v", tc.direction, got, want)
		}
	}
}

func TestKernelGraphKeepsStructureOnly(t *testing.T) {
	t.Parallel()

	width := 4.0
	label := "corridor"

	got := kernelGraphOf(domain.GraphDocument{
		Nodes: []domain.Node{
			{ID: "gate", Type: domain.NodeTypeBoundary, Labels: map[string]string{"ja": "門"}, GroupID: "g1", Layout: domain.Layout{X: 1, Y: 2, Width: &width}},
			{ID: "hall", Type: domain.NodeTypeGoal, GroupID: "g1"},
			{ID: "aisle", Type: domain.NodeTypeTransitOnly},
		},
		Groups: []domain.Group{{ID: "g1", Labels: map[string]string{"ja": "東"}, Layout: domain.Layout{X: 0, Y: 0, Width: &width}}},
		Edges: []domain.Edge{
			{ID: "e2", SourceNodeID: "hall", TargetNodeID: "aisle", Direction: domain.EdgeDirectionBothWays, Label: &label},
			{ID: "e1", SourceNodeID: "gate", TargetNodeID: "hall", Direction: domain.EdgeDirectionOneWay},
		},
	})

	want := domain.KernelGraph{
		EventPublicID: "fedcba9876543210",
		RevisionID:    "0123456789abcdef",
		Points: []domain.Point{
			{ID: "gate", Type: domain.PointTypeTransitOnly, IsBoundary: true, BoundaryActive: true},
			{ID: "hall", Type: domain.PointTypeGoal, IsBoundary: false, BoundaryActive: true},
			{ID: "aisle", Type: domain.PointTypeTransitOnly, IsBoundary: false, BoundaryActive: true},
		},
		Routes: []domain.Route{
			{ID: "e2", FromPointID: "hall", ToPointID: "aisle", Direction: domain.DirectionAttributeBothWays},
			{ID: "e1", FromPointID: "gate", ToPointID: "hall", Direction: domain.DirectionAttributeOneWay},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("KernelGraph() = %+v, want %+v", got, want)
	}
}
