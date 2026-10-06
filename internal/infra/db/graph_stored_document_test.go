package db

import (
	"database/sql"
	"database/sql/driver"
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

func TestStoredDocumentRoundTrip(t *testing.T) {
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

			stored, err := newStoredDocument(tt.document)
			if err != nil {
				t.Fatalf("newStoredDocument() error = %v", err)
			}

			var restored storedDocument

			for _, column := range []struct {
				from driver.Valuer
				to   sql.Scanner
			}{
				{stored.Kernel, &restored.Kernel},
				{stored.Labels, &restored.Labels},
				{stored.Layout, &restored.Layout},
			} {
				value, err := column.from.Value()
				if err != nil {
					t.Fatalf("Value() error = %v", err)
				}

				if err := column.to.Scan(value); err != nil {
					t.Fatalf("Scan() error = %v", err)
				}
			}

			got := restored.graphDocument()
			if !reflect.DeepEqual(got, tt.document) {
				t.Errorf("stored document round trip = %#v, want %#v", got, tt.document)
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

func TestNewStoredDocumentRejectsDuplicateIDs(t *testing.T) {
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

			if _, err := newStoredDocument(tt.document); !errors.Is(err, domain.ErrInvalidGraphDocument) {
				t.Errorf("newStoredDocument() error = %v, want %v", err, domain.ErrInvalidGraphDocument)
			}
		})
	}
}
