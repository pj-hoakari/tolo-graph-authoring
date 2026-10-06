package db

import (
	"errors"
	"reflect"
	"testing"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func richDocument() domain.GraphDocument {
	return domain.GraphDocument{
		Nodes: []domain.Node{
			{
				ID: "n2", Type: domain.NodeTypeGoal, Labels: map[string]string{"ja": "正門", "en": "Main gate"},
				GroupID: "g1", Layout: domain.Layout{X: 1.5, Y: -2.25, Width: ptr(10.0), Height: ptr(0.1)},
			},
			{ID: "n1", Type: domain.NodeTypeBoundary, Labels: nil, GroupID: "", Layout: domain.Layout{X: 0, Y: 3}},
			{ID: "n3", Type: domain.NodeTypeTransitOnly, Labels: map[string]string{}, Layout: domain.Layout{Width: ptr(2.0)}},
		},
		Groups: []domain.Group{
			{ID: "g1", Labels: map[string]string{"ja": "東棟"}, Layout: domain.Layout{X: 5, Y: 6, Width: ptr(7.0), Height: ptr(8.0)}},
			{ID: "g0", Labels: nil},
		},
		Edges: []domain.Edge{
			{ID: "e2", SourceNodeID: "n2", TargetNodeID: "n1", Direction: domain.EdgeDirectionOneWay, Label: ptr("階段")},
			{ID: "e1", SourceNodeID: "n1", TargetNodeID: "n3", Direction: domain.EdgeDirectionBothWays, Label: nil},
			{ID: "e3", SourceNodeID: "n3", TargetNodeID: "n2", Direction: domain.EdgeDirectionOneWay, Label: ptr("")},
		},
	}
}

func TestDraftColumnsRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document domain.GraphDocument
	}{
		{"rich document", richDocument()},
		{"nil document", domain.GraphDocument{}},
		{"empty document", domain.GraphDocument{Nodes: []domain.Node{}, Groups: []domain.Group{}, Edges: []domain.Edge{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			columns, err := encodeDraft(tt.document)
			if err != nil {
				t.Fatalf("encodeDraft() error = %v", err)
			}

			got, err := decodeDraft(columns)
			if err != nil {
				t.Fatalf("decodeDraft() error = %v", err)
			}

			if !reflect.DeepEqual(got, tt.document) {
				t.Errorf("decodeDraft(encodeDraft(d)) = %#v, want %#v", got, tt.document)
			}

			before, err := domain.NewVenueGraph("t", "e", tt.document)
			if err != nil {
				t.Fatalf("NewVenueGraph() error = %v", err)
			}

			after, err := domain.NewVenueGraph("t", "e", got)
			if err != nil {
				t.Fatalf("NewVenueGraph() error = %v", err)
			}

			if after.DraftRevisionID() != before.DraftRevisionID() {
				t.Errorf("DraftRevisionID() after round trip = %q, want %q", after.DraftRevisionID(), before.DraftRevisionID())
			}
		})
	}
}

func TestEncodeDraftRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document domain.GraphDocument
	}{
		{"nodes", domain.GraphDocument{Nodes: []domain.Node{{ID: "n1"}, {ID: "n1"}}}},
		{"groups", domain.GraphDocument{Groups: []domain.Group{{ID: "g1"}, {ID: "g1"}}}},
		{"edges", domain.GraphDocument{Edges: []domain.Edge{{ID: "e1"}, {ID: "e1"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := encodeDraft(tt.document); !errors.Is(err, domain.ErrInvalidGraphDocument) {
				t.Errorf("encodeDraft() error = %v, want %v", err, domain.ErrInvalidGraphDocument)
			}
		})
	}
}
