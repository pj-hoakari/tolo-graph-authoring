package db

import (
	"context"
	"errors"
	"reflect"
	"testing"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
)

func editingContext() context.Context {
	return internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
		TokenUse:       internaljwt.TokenUseEventAccess,
		TenantPublicID: ownerTenant,
		EventPublicID:  graphEvent,
	})
}

func placedGraph(t *testing.T) (*PostgresGraphRepository, *application.GraphService, domain.Graph) {
	t.Helper()

	repo := newTestGraphRepository(t)
	service := application.NewGraphService(repo, repo, callerTenantEvents{})

	document := domain.GraphDocument{
		Nodes: []domain.Node{{ID: "gate", Type: domain.NodeTypeBoundary}, {ID: "hall", Type: domain.NodeTypeGoal}},
		Edges: []domain.Edge{{ID: "e1", SourceNodeID: "gate", TargetNodeID: "hall", Direction: domain.EdgeDirectionBothWays}},
	}

	graph, err := service.SaveGraph(editingContext(), application.SaveGraphInput{EventPublicID: graphEvent, Document: &document})
	if err != nil {
		t.Fatalf("SaveGraph() error = %v", err)
	}

	return repo, service, graph
}

func findPlacements(t *testing.T, repo *PostgresGraphRepository, graph domain.Graph) domain.Placements {
	t.Helper()

	placements, err := repo.FindPlacements(context.Background(), graph)
	if err != nil {
		t.Fatalf("FindPlacements() error = %v", err)
	}

	return placements
}

func TestQrLocationIsAddedUpdatedAndRemovedThroughService(t *testing.T) {
	repo, service, graph := placedGraph(t)
	ctx := editingContext()

	added, err := service.AddQrLocation(ctx, application.AddQrLocationInput{
		EventPublicID: graphEvent,
		QrLocation:    domain.QrLocation{Name: "Gate poster", Kind: "poster", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "gate"}},
	})
	if err != nil {
		t.Fatalf("AddQrLocation() error = %v", err)
	}

	moved := domain.QrLocation{ID: added.ID, Name: "Corridor sign", Kind: "sign", Anchor: routeMapping("", "e1", 0.5).Anchor}
	if _, err := service.UpdateQrLocation(ctx, application.UpdateQrLocationInput{EventPublicID: graphEvent, QrLocation: moved}); err != nil {
		t.Fatalf("UpdateQrLocation() error = %v", err)
	}

	if got, want := findPlacements(t, repo, graph), (domain.Placements{Mappings: []domain.ObservationPointMapping{}, QrLocations: []domain.QrLocation{moved}}); !reflect.DeepEqual(got, want) {
		t.Errorf("FindPlacements() = %+v, want %+v", got, want)
	}

	remove := application.RemoveQrLocationInput{EventPublicID: graphEvent, QrLocationID: added.ID}
	if err := service.RemoveQrLocation(ctx, remove); err != nil {
		t.Fatalf("RemoveQrLocation() error = %v", err)
	}

	if err := service.RemoveQrLocation(ctx, remove); !errors.Is(err, repository.ErrQrLocationNotFound) {
		t.Errorf("RemoveQrLocation() of a removed location error = %v, want %v", err, repository.ErrQrLocationNotFound)
	}

	if got := findPlacements(t, repo, graph); len(got.QrLocations) != 0 {
		t.Errorf("FindPlacements() QR locations = %+v, want none", got.QrLocations)
	}
}

func TestPlacementsKeepTheirElementsAndDoNotShareThem(t *testing.T) {
	_, service, _ := placedGraph(t)
	ctx := editingContext()

	if _, err := service.MapObservationPoint(ctx, application.MapObservationPointInput{EventPublicID: graphEvent, Mapping: routeMapping("cam-1", "e1", 0.5)}); err != nil {
		t.Fatalf("MapObservationPoint() error = %v", err)
	}

	onRoute := domain.QrLocation{Name: "Corridor sign", Kind: "sign", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindRoute, ElementID: "e1"}}
	if _, err := service.AddQrLocation(ctx, application.AddQrLocationInput{EventPublicID: graphEvent, QrLocation: onRoute}); !errors.Is(err, domain.ErrPlacementConflict) {
		t.Errorf("AddQrLocation() on a mapped route error = %v, want %v", err, domain.ErrPlacementConflict)
	}

	withoutRoute := domain.GraphDocument{
		Nodes: []domain.Node{{ID: "gate", Type: domain.NodeTypeBoundary}, {ID: "hall", Type: domain.NodeTypeGoal}},
	}
	if _, err := service.SaveGraph(ctx, application.SaveGraphInput{EventPublicID: graphEvent, Document: &withoutRoute}); !errors.Is(err, domain.ErrAnchorTargetNotFound) {
		t.Errorf("SaveGraph() without the mapped route error = %v, want %v", err, domain.ErrAnchorTargetNotFound)
	}
}

func TestPostgresGraphRepositoryQrLocationsKeepOtherTenantOut(t *testing.T) {
	repo := newTestGraphRepository(t)
	ctx := context.Background()

	owned := saveGraph(t, repo, singleNode("n1"))
	location := domain.QrLocation{ID: "0123456789abcdef", Name: "Poster", Kind: "poster", Anchor: domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "n1"}}

	if err := repo.AddQrLocation(ctx, owned, location); err != nil {
		t.Fatalf("AddQrLocation() error = %v", err)
	}

	intruder := newGraph(t, otherTenant, singleNode("n1"))
	if err := repo.AddQrLocation(ctx, intruder, domain.QrLocation{ID: "fedcba9876543210", Name: "x", Kind: "x", Anchor: location.Anchor}); err == nil {
		t.Error("AddQrLocation() by another tenant error = nil, want a foreign key violation")
	}

	renamed := location
	renamed.Name = "Hijacked"

	if err := repo.UpdateQrLocation(ctx, intruder, renamed); !errors.Is(err, repository.ErrQrLocationNotFound) {
		t.Errorf("UpdateQrLocation() by another tenant error = %v, want %v", err, repository.ErrQrLocationNotFound)
	}

	if err := repo.RemoveQrLocation(ctx, intruder, location.ID); !errors.Is(err, repository.ErrQrLocationNotFound) {
		t.Errorf("RemoveQrLocation() by another tenant error = %v, want %v", err, repository.ErrQrLocationNotFound)
	}

	if got := findPlacements(t, repo, intruder); len(got.QrLocations) != 0 {
		t.Errorf("FindPlacements() for another tenant = %+v, want none", got.QrLocations)
	}

	if got := findPlacements(t, repo, owned); !reflect.DeepEqual(got.QrLocations, []domain.QrLocation{location}) {
		t.Errorf("FindPlacements() QR locations = %+v, want %+v", got.QrLocations, []domain.QrLocation{location})
	}
}
