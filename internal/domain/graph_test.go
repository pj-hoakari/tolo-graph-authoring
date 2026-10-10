package domain_test

import (
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func venue() domain.GraphDocument {
	return domain.GraphDocument{
		Nodes: []domain.Node{
			{ID: "stage", Type: domain.NodeTypeGoal, Labels: map[string]string{"ja": "舞台", "en": "Stage"}, GroupID: "hall", Layout: domain.Layout{X: 40, Y: 20}},
			{ID: "south", Type: domain.NodeTypeExternal, Labels: map[string]string{"ja": "南口"}, Layout: domain.Layout{X: 0, Y: 300}},
			{ID: "gate", Type: domain.NodeTypeTransitOnly, Labels: nil, GroupID: "floor", Layout: domain.Layout{X: 10, Y: 200, Width: ptr(12.0)}},
			{ID: "north", Type: domain.NodeTypeExternal, Layout: domain.Layout{X: 0, Y: -300}},
			{ID: "lobby", Type: domain.NodeTypeGoalTransitMixed, Labels: map[string]string{}, Layout: domain.Layout{X: 5, Y: 5}},
		},
		Groups: []domain.Group{
			{ID: "hall", Labels: map[string]string{"ja": "ホール"}, ParentGroupID: "floor", Layout: domain.Layout{X: 30, Y: 10, Width: ptr(100.0), Height: ptr(80.0)}},
			{ID: "floor", MinWidth: ptr(300.0), MinHeight: ptr(250.0), Layout: domain.Layout{X: 0, Y: 0, Width: ptr(320.0), Height: ptr(260.0)}},
		},
		Edges: []domain.Edge{
			{ID: "walk", SourceNodeID: "gate", TargetNodeID: "stage", Direction: domain.EdgeDirectionBothWays, Label: ptr("通路")},
			{ID: "entry", SourceNodeID: "south", TargetNodeID: "gate", Direction: domain.EdgeDirectionOneWay, Label: ptr("入場")},
			{ID: "exit", SourceNodeID: "lobby", TargetNodeID: "north", Direction: domain.EdgeDirectionOneWay},
			{ID: "aisle", SourceNodeID: "lobby", TargetNodeID: "stage", Direction: domain.EdgeDirectionOneWay, Label: ptr("")},
		},
	}
}

func newGraph(t *testing.T, document domain.GraphDocument) domain.Graph {
	t.Helper()

	graph, err := domain.NewGraph("a1b2c3d4e5f60718", "fedcba9876543210", document)
	if err != nil {
		t.Fatalf("NewGraph() error = %v", err)
	}

	return graph
}

func TestDraftIsTheDocumentInCanonicalOrderAndSurvivesItsParts(t *testing.T) {
	t.Parallel()

	want := venue()
	slices.SortFunc(want.Nodes, func(a, b domain.Node) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(want.Groups, func(a, b domain.Group) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(want.Edges, func(a, b domain.Edge) int { return strings.Compare(a.ID, b.ID) })

	graph := newGraph(t, venue())
	if got := graph.Draft(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Draft() = %+v, want %+v", got, want)
	}

	restored, err := domain.RestoreGraph(graph.TenantPublicID(), graph.EventPublicID(), graph.DraftParts(), "")
	if err != nil {
		t.Fatalf("RestoreGraph() error = %v", err)
	}

	if got := restored.Draft(); !reflect.DeepEqual(got, want) {
		t.Errorf("Draft() restored from parts = %+v, want %+v", got, want)
	}

	if restored.DraftRevisionID() != graph.DraftRevisionID() {
		t.Errorf("DraftRevisionID() restored from parts = %q, want %q", restored.DraftRevisionID(), graph.DraftRevisionID())
	}
}

func TestKernelExcludesExternalNodesAndMarksTheirNeighboursAsBoundary(t *testing.T) {
	t.Parallel()

	got := venue().Parts().Kernel
	want := domain.GraphKernel{
		Points: []domain.Point{
			{ID: "gate", Type: domain.PointTypeTransitOnly, Boundary: &domain.PointBoundary{Direction: domain.BoundaryDirectionEntry, Active: true}},
			{ID: "lobby", Type: domain.PointTypeGoalTransitMixed, Boundary: &domain.PointBoundary{Direction: domain.BoundaryDirectionExit, Active: true}},
			{ID: "stage", Type: domain.PointTypeGoal, Boundary: nil},
		},
		Routes: []domain.Route{
			{ID: "aisle", FromPointID: "lobby", ToPointID: "stage", Direction: domain.DirectionAttributeOneWay},
			{ID: "walk", FromPointID: "gate", ToPointID: "stage", Direction: domain.DirectionAttributeBothWays},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parts().Kernel = %+v, want %+v", got, want)
	}
}

func TestBoundaryDirectionFollowsEdgesToExternals(t *testing.T) {
	t.Parallel()

	oneWay, bothWays := domain.EdgeDirectionOneWay, domain.EdgeDirectionBothWays
	edge := func(id, source, target string, direction domain.EdgeDirection) domain.Edge {
		return domain.Edge{ID: id, SourceNodeID: source, TargetNodeID: target, Direction: direction}
	}

	boundary := func(direction domain.BoundaryDirection) *domain.PointBoundary {
		return &domain.PointBoundary{Direction: direction, Active: true}
	}

	for _, tc := range []struct {
		name      string
		nodeType  domain.NodeType
		edges     []domain.Edge
		pointType domain.PointType
		want      *domain.PointBoundary
	}{
		{"not connected", domain.NodeTypeGoal, nil, domain.PointTypeGoal, nil},
		{"one way from an external", domain.NodeTypeGoal, []domain.Edge{edge("e", "x", "p", oneWay)}, domain.PointTypeGoal, boundary(domain.BoundaryDirectionEntry)},
		{"one way to an external", domain.NodeTypeGoalTransitMixed, []domain.Edge{edge("e", "p", "y", oneWay)}, domain.PointTypeGoalTransitMixed, boundary(domain.BoundaryDirectionExit)},
		{"both ways from an external", domain.NodeTypeTransitOnly, []domain.Edge{edge("e", "x", "p", bothWays)}, domain.PointTypeTransitOnly, boundary(domain.BoundaryDirectionEntryAndExit)},
		{"both ways to an external", domain.NodeTypeTransitOnly, []domain.Edge{edge("e", "p", "y", bothWays)}, domain.PointTypeTransitOnly, boundary(domain.BoundaryDirectionEntryAndExit)},
		{"entry and exit through two externals", domain.NodeTypeGoal, []domain.Edge{edge("in", "x", "p", oneWay), edge("out", "p", "y", oneWay)}, domain.PointTypeGoal, boundary(domain.BoundaryDirectionEntryAndExit)},
		{"two entries", domain.NodeTypeGoal, []domain.Edge{edge("a", "x", "p", oneWay), edge("b", "y", "p", oneWay)}, domain.PointTypeGoal, boundary(domain.BoundaryDirectionEntry)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := domain.GraphDocument{
				Nodes: []domain.Node{{ID: "p", Type: tc.nodeType}, {ID: "x", Type: domain.NodeTypeExternal}, {ID: "y", Type: domain.NodeTypeExternal}},
				Edges: tc.edges,
			}.Parts().Kernel.Points
			want := []domain.Point{{ID: "p", Type: tc.pointType, Boundary: tc.want}}

			if !reflect.DeepEqual(got, want) {
				t.Errorf("Parts().Kernel.Points = %+v, want %+v", got, want)
			}
		})
	}
}

func TestWithDraftRejectsStructurallyInvalidDocuments(t *testing.T) {
	t.Parallel()

	goal := func(id string) domain.Node { return domain.Node{ID: id, Type: domain.NodeTypeGoal} }
	external := func(id string) domain.Node { return domain.Node{ID: id, Type: domain.NodeTypeExternal} }
	edge := func(id, source, target string) domain.Edge {
		return domain.Edge{ID: id, SourceNodeID: source, TargetNodeID: target, Direction: domain.EdgeDirectionOneWay}
	}

	for _, tc := range []struct {
		name     string
		document domain.GraphDocument
	}{
		{"duplicate node ID", domain.GraphDocument{Nodes: []domain.Node{goal("a"), goal("a")}}},
		{"duplicate group ID", domain.GraphDocument{Groups: []domain.Group{{ID: "g"}, {ID: "g"}}}},
		{"duplicate edge ID", domain.GraphDocument{Nodes: []domain.Node{goal("a"), goal("b")}, Edges: []domain.Edge{edge("e", "a", "b"), edge("e", "b", "a")}}},
		{"unspecified node type", domain.GraphDocument{Nodes: []domain.Node{{ID: "a"}}}},
		{"unspecified edge direction", domain.GraphDocument{Nodes: []domain.Node{goal("a"), goal("b")}, Edges: []domain.Edge{{ID: "e", SourceNodeID: "a", TargetNodeID: "b"}}}},
		{"dangling edge source", domain.GraphDocument{Nodes: []domain.Node{goal("b")}, Edges: []domain.Edge{edge("e", "a", "b")}}},
		{"dangling edge target", domain.GraphDocument{Nodes: []domain.Node{goal("a")}, Edges: []domain.Edge{edge("e", "a", "b")}}},
		{"node in missing group", domain.GraphDocument{Nodes: []domain.Node{{ID: "a", Type: domain.NodeTypeGoal, GroupID: "g"}}}},
		{"missing parent group", domain.GraphDocument{Groups: []domain.Group{{ID: "g", ParentGroupID: "missing"}}}},
		{"group is its own parent", domain.GraphDocument{Groups: []domain.Group{{ID: "g", ParentGroupID: "g"}}}},
		{"group parent cycle", domain.GraphDocument{Groups: []domain.Group{{ID: "a", ParentGroupID: "b"}, {ID: "b", ParentGroupID: "c"}, {ID: "c", ParentGroupID: "a"}}}},
		{"cycle above a valid group", domain.GraphDocument{Groups: []domain.Group{{ID: "leaf", ParentGroupID: "a"}, {ID: "a", ParentGroupID: "b"}, {ID: "b", ParentGroupID: "a"}}}},
		{"external node in a group", domain.GraphDocument{Nodes: []domain.Node{{ID: "x", Type: domain.NodeTypeExternal, GroupID: "g"}}, Groups: []domain.Group{{ID: "g"}}}},
		{"non-finite node layout", domain.GraphDocument{Nodes: []domain.Node{{ID: "a", Type: domain.NodeTypeGoal, Layout: domain.Layout{X: math.Inf(1)}}}}},
		{"non-finite group minimum size", domain.GraphDocument{Groups: []domain.Group{{ID: "g", MinWidth: ptr(math.NaN())}}}},
		{"edge between external nodes", domain.GraphDocument{Nodes: []domain.Node{external("x"), external("y")}, Edges: []domain.Edge{edge("e", "x", "y")}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := domain.NewGraph("a1b2c3d4e5f60718", "fedcba9876543210", tc.document); !errors.Is(err, domain.ErrInvalidGraphDocument) {
				t.Errorf("NewGraph() error = %v, want %v", err, domain.ErrInvalidGraphDocument)
			}
		})
	}

	nested := domain.GraphDocument{
		Nodes:  []domain.Node{{ID: "a", Type: domain.NodeTypeGoal, GroupID: "c"}, external("x"), external("y")},
		Groups: []domain.Group{{ID: "a"}, {ID: "b", ParentGroupID: "a"}, {ID: "c", ParentGroupID: "b"}},
		Edges:  []domain.Edge{edge("in", "x", "a"), edge("out", "a", "y")},
	}
	if _, err := domain.NewGraph("a1b2c3d4e5f60718", "fedcba9876543210", nested); err != nil {
		t.Errorf("NewGraph() of deeply nested groups and several externals error = %v, want nil", err)
	}
}

func TestDraftRevisionIDIgnoresLayout(t *testing.T) {
	t.Parallel()

	base := newGraph(t, venue()).DraftRevisionID()

	for _, tc := range []struct {
		name string
		edit func(*domain.GraphDocument)
	}{
		{"node moved", func(d *domain.GraphDocument) { d.Nodes[0].Layout = domain.Layout{X: 999, Y: 999} }},
		{"node regrouped", func(d *domain.GraphDocument) { d.Nodes[0].GroupID = "floor" }},
		{"group resized", func(d *domain.GraphDocument) { d.Groups[1].Layout.Width = ptr(500.0); d.Groups[1].MinWidth = ptr(400.0) }},
		{"group unnested", func(d *domain.GraphDocument) { d.Groups[0].ParentGroupID = "" }},
		{"external moved and renamed", func(d *domain.GraphDocument) {
			d.Nodes[1].Layout = domain.Layout{X: 1, Y: 1}
			d.Nodes[1].Labels = map[string]string{"ja": "東口"}
		}},
		{"external edge moved to another external", func(d *domain.GraphDocument) { d.Edges[1].SourceNodeID = "north" }},
		{"external edge renamed", func(d *domain.GraphDocument) { d.Edges[2].Label = ptr("出口") }},
		{"elements reordered", func(d *domain.GraphDocument) {
			slices.Reverse(d.Nodes)
			slices.Reverse(d.Groups)
			slices.Reverse(d.Edges)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			document := venue()
			tc.edit(&document)

			if got := newGraph(t, document).DraftRevisionID(); got != base {
				t.Errorf("DraftRevisionID() = %q, want unchanged %q", got, base)
			}
		})
	}
}

func TestDraftRevisionIDFollowsKernelAndLabels(t *testing.T) {
	t.Parallel()

	base := newGraph(t, venue()).DraftRevisionID()

	for _, tc := range []struct {
		name string
		edit func(*domain.GraphDocument)
	}{
		{"external connection added", func(d *domain.GraphDocument) {
			d.Edges = append(d.Edges, domain.Edge{ID: "side", SourceNodeID: "north", TargetNodeID: "lobby", Direction: domain.EdgeDirectionOneWay})
		}},
		{"external connection removed", func(d *domain.GraphDocument) { d.Edges = slices.Delete(d.Edges, 1, 3) }},
		{"external edge made both ways", func(d *domain.GraphDocument) { d.Edges[1].Direction = domain.EdgeDirectionBothWays }},
		{"external edge reversed", func(d *domain.GraphDocument) {
			d.Edges[2].SourceNodeID, d.Edges[2].TargetNodeID = d.Edges[2].TargetNodeID, d.Edges[2].SourceNodeID
		}},
		{"point type changed", func(d *domain.GraphDocument) { d.Nodes[0].Type = domain.NodeTypeGoalTransitMixed }},
		{"point renamed", func(d *domain.GraphDocument) { d.Nodes[0].Labels = map[string]string{"ja": "大舞台"} }},
		{"group renamed", func(d *domain.GraphDocument) { d.Groups[0].Labels = map[string]string{"ja": "大ホール"} }},
		{"route renamed", func(d *domain.GraphDocument) { d.Edges[0].Label = ptr("廊下") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			document := venue()
			tc.edit(&document)

			if got := newGraph(t, document).DraftRevisionID(); got == base {
				t.Errorf("DraftRevisionID() = %q, want a new revision", got)
			}
		})
	}
}

func TestPublishedRevisionServesItsKernel(t *testing.T) {
	t.Parallel()

	kernel := venue().Parts().Kernel
	got := domain.PublishedRevision{
		TenantPublicID: "a1b2c3d4e5f60718",
		EventPublicID:  "fedcba9876543210",
		RevisionID:     "0123456789abcdef",
		Kernel:         kernel,
	}.KernelGraph()
	want := domain.KernelGraph{EventPublicID: "fedcba9876543210", RevisionID: "0123456789abcdef", Points: kernel.Points, Routes: kernel.Routes}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("KernelGraph() = %+v, want %+v", got, want)
	}
}
