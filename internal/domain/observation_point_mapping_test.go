package domain_test

import (
	"errors"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

func TestVerifyMappingAcceptsOnlyElementsOfTheDraft(t *testing.T) {
	t.Parallel()

	graph, err := domain.NewGraph("a1b2c3d4e5f60718", "fedcba9876543210", domain.GraphDocument{
		Nodes: []domain.Node{
			{ID: "gate", Type: domain.NodeTypeTransitOnly},
			{ID: "hall", Type: domain.NodeTypeGoal},
			{ID: "outside", Type: domain.NodeTypeExternal},
		},
		Edges: []domain.Edge{
			{ID: "e1", SourceNodeID: "gate", TargetNodeID: "hall", Direction: domain.EdgeDirectionOneWay},
			{ID: "entry", SourceNodeID: "outside", TargetNodeID: "gate", Direction: domain.EdgeDirectionOneWay},
		},
	})
	if err != nil {
		t.Fatalf("NewGraph() error = %v", err)
	}

	draft := domain.DraftKernel{GraphRef: graph.Ref(), Kernel: graph.DraftParts().Kernel}

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
		{"external node", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "outside"}, domain.ErrAnchorTargetNotFound},
		{"edge to an external node", "cam-1", domain.GraphAnchor{Kind: domain.AnchorKindRoute, ElementID: "entry"}, domain.ErrAnchorTargetNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mapping := domain.ObservationPointMapping{ObservationPointID: tc.observationPointID, Anchor: tc.anchor}
			if err := draft.VerifyMapping(mapping); !errors.Is(err, tc.want) {
				t.Errorf("VerifyMapping() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func venuePlacements() domain.Placements {
	return domain.Placements{
		Mappings: []domain.ObservationPointMapping{
			{ObservationPointID: "cam-1", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "gate"}},
			{ObservationPointID: "cam-2", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindRoute, ElementID: "walk", RoutePosition: ptr(0.5)}},
		},
		QrLocations: []domain.QrLocation{
			{ID: "qr-1", Name: "入口", Kind: "entrance", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "lobby"}},
			{ID: "qr-2", Name: "舞台", Kind: "stage", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "stage"}},
		},
	}
}

func revisionID(t *testing.T, documentHash string, placements domain.Placements) string {
	t.Helper()

	id, err := domain.RevisionID(documentHash, placements)
	if err != nil {
		t.Fatalf("RevisionID() error = %v", err)
	}

	return id
}

func TestRevisionIDFollowsPlacements(t *testing.T) {
	t.Parallel()

	documentHash := newGraph(t, venue()).DocumentHash()
	base := revisionID(t, documentHash, venuePlacements())

	for _, tc := range []struct {
		name string
		edit func(*domain.Placements)
	}{
		{"mapping added", func(p *domain.Placements) {
			p.Mappings = append(p.Mappings, domain.ObservationPointMapping{ObservationPointID: "cam-3", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "stage"}})
		}},
		{"mapping moved", func(p *domain.Placements) { p.Mappings[1].Anchor.RoutePosition = ptr(0.25) }},
		{"mapping removed", func(p *domain.Placements) { p.Mappings = p.Mappings[:1] }},
		{"QR location added", func(p *domain.Placements) {
			p.QrLocations = append(p.QrLocations, domain.QrLocation{ID: "qr-3", Name: "出口", Kind: "exit", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "gate"}})
		}},
		{"QR location renamed", func(p *domain.Placements) { p.QrLocations[0].Name = "正面入口" }},
		{"QR location removed", func(p *domain.Placements) { p.QrLocations = p.QrLocations[1:] }},
		{"QR location removed and added again", func(p *domain.Placements) { p.QrLocations[0].ID = "qr-9" }},
		{"all placements removed", func(p *domain.Placements) { *p = domain.Placements{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			placements := venuePlacements()
			tc.edit(&placements)

			if got := revisionID(t, documentHash, placements); got == base {
				t.Errorf("RevisionID() = %q, want a new revision", got)
			}
		})
	}

	if got := revisionID(t, "0123456789abcdef", venuePlacements()); got == base {
		t.Errorf("RevisionID() with another document = %q, want a new revision", got)
	}
}

func TestRevisionIDIgnoresPlacementOrder(t *testing.T) {
	t.Parallel()

	documentHash := newGraph(t, venue()).DocumentHash()
	base := revisionID(t, documentHash, venuePlacements())

	placements := venuePlacements()
	slices.Reverse(placements.Mappings)
	slices.Reverse(placements.QrLocations)

	if got := revisionID(t, documentHash, placements); got != base {
		t.Errorf("RevisionID() of reordered placements = %q, want %q", got, base)
	}

	if empty, none := revisionID(t, documentHash, domain.Placements{}), revisionID(t, documentHash, domain.Placements{
		Mappings: []domain.ObservationPointMapping{}, QrLocations: []domain.QrLocation{},
	}); empty != none {
		t.Errorf("RevisionID() of nil placements = %q, want %q as for empty placements", empty, none)
	}
}

func TestDraftRevisionIDIncludesThePlacementsOfTheGraph(t *testing.T) {
	t.Parallel()

	graph := newGraph(t, venue())

	restored, err := domain.RestoreGraph(graph.TenantPublicID(), graph.EventPublicID(), graph.DraftParts(), venuePlacements(), "")
	if err != nil {
		t.Fatalf("RestoreGraph() error = %v", err)
	}

	if want := revisionID(t, graph.DocumentHash(), venuePlacements()); restored.DraftRevisionID() != want {
		t.Fatalf("DraftRevisionID() = %q, want %q", restored.DraftRevisionID(), want)
	}

	if restored.DraftRevisionID() == graph.DraftRevisionID() {
		t.Fatalf("DraftRevisionID() with placements = %q, want it to differ from the graph without placements", restored.DraftRevisionID())
	}

	moved := venue()
	moved.Nodes[0].Layout = domain.Layout{X: 999, Y: 999}

	relaidOut, err := restored.WithDraft(moved)
	if err != nil {
		t.Fatalf("WithDraft() error = %v", err)
	}

	if relaidOut.DraftRevisionID() != restored.DraftRevisionID() {
		t.Errorf("DraftRevisionID() after a layout change = %q, want unchanged %q", relaidOut.DraftRevisionID(), restored.DraftRevisionID())
	}

	if !reflect.DeepEqual(relaidOut.Placements(), venuePlacements()) {
		t.Errorf("Placements() after WithDraft() = %+v, want %+v", relaidOut.Placements(), venuePlacements())
	}
}

func TestWithDraftKeepsEveryPlacementOnTheGraph(t *testing.T) {
	t.Parallel()

	graph, err := domain.RestoreGraph("a1b2c3d4e5f60718", "fedcba9876543210", venue().Parts(), venuePlacements(), "")
	if err != nil {
		t.Fatalf("RestoreGraph() error = %v", err)
	}

	for _, tc := range []struct {
		name string
		edit func(*domain.GraphDocument)
	}{
		{"mapped point removed", func(d *domain.GraphDocument) {
			d.Nodes = slices.DeleteFunc(d.Nodes, func(n domain.Node) bool { return n.ID == "gate" || n.ID == "south" })
			d.Edges = slices.DeleteFunc(d.Edges, func(e domain.Edge) bool { return e.ID == "walk" || e.ID == "entry" })
		}},
		{"mapped route removed", func(d *domain.GraphDocument) {
			d.Edges[0] = domain.Edge{ID: "corridor", SourceNodeID: "gate", TargetNodeID: "stage", Direction: domain.EdgeDirectionBothWays}
		}},
		{"QR point renamed", func(d *domain.GraphDocument) {
			d.Nodes[4].ID, d.Edges[2].SourceNodeID, d.Edges[3].SourceNodeID = "foyer", "foyer", "foyer"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			document := venue()
			tc.edit(&document)

			if _, err := graph.WithDraft(document); !errors.Is(err, domain.ErrAnchorTargetNotFound) {
				t.Errorf("WithDraft() error = %v, want %v", err, domain.ErrAnchorTargetNotFound)
			}
		})
	}
}
