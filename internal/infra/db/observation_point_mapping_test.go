package db

import (
	"context"
	"errors"
	"reflect"
	"testing"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/tenantctx"
)

func routeMapping(observationPointID, routeID string, position float64) domain.ObservationPointMapping {
	return domain.ObservationPointMapping{
		ObservationPointID: observationPointID,
		Anchor:             domain.GraphAnchor{Kind: domain.AnchorKindRoute, ElementID: routeID, RoutePosition: &position},
	}
}

func TestObservationPointMappedThroughServiceIsSupplied(t *testing.T) {
	repo := newTestGraphRepository(t)
	service := application.NewGraphService(repo, repo, callerTenantEvents{})
	editing := internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
		TokenUse:       internaljwt.TokenUseEventAccess,
		TenantPublicID: ownerTenant,
		EventPublicID:  graphEvent,
	})

	document := domain.GraphDocument{
		Nodes: []domain.Node{{ID: "gate", Type: domain.NodeTypeTransitOnly}, {ID: "hall", Type: domain.NodeTypeGoal}},
		Edges: []domain.Edge{{ID: "e1", SourceNodeID: "gate", TargetNodeID: "hall", Direction: domain.EdgeDirectionBothWays}},
	}

	if _, err := service.SaveGraph(editing, application.SaveGraphInput{EventPublicID: graphEvent, Document: &document}); err != nil {
		t.Fatalf("SaveGraph() error = %v", err)
	}

	published, err := service.PublishRevision(editing, application.PublishRevisionInput{EventPublicID: graphEvent})
	if err != nil {
		t.Fatalf("PublishRevision() error = %v", err)
	}

	pointMapping := domain.ObservationPointMapping{
		ObservationPointID: "cam-2",
		Anchor:             domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "gate"},
	}

	for _, mapping := range []domain.ObservationPointMapping{pointMapping, routeMapping("cam-1", "e1", 0.25), routeMapping("cam-1", "e1", 0.75)} {
		if _, err := service.MapObservationPoint(editing, application.MapObservationPointInput{EventPublicID: graphEvent, Mapping: mapping}); err != nil {
			t.Fatalf("MapObservationPoint(%+v) error = %v", mapping, err)
		}
	}

	got, err := service.GetObservationPointMappings(context.Background(), application.GetObservationPointMappingsInput{EventPublicID: graphEvent})
	if err != nil {
		t.Fatalf("GetObservationPointMappings() error = %v", err)
	}

	want := application.ObservationPointMappings{
		RevisionID: published.RevisionID(),
		Mappings:   []domain.ObservationPointMapping{routeMapping("cam-1", "e1", 0.75), pointMapping},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetObservationPointMappings() = %+v, want %+v", got, want)
	}
}

func TestPostgresGraphRepositorySaveObservationPointMappingKeepsOtherTenantOut(t *testing.T) {
	repo := newTestGraphRepository(t)
	ctx := context.Background()

	owned := saveGraph(t, repo, singleNode("n1"))
	if err := repo.SaveObservationPointMapping(ctx, owned, routeMapping("cam-1", "e1", 0.5)); err != nil {
		t.Fatalf("SaveObservationPointMapping() error = %v", err)
	}

	intruder := newGraph(t, otherTenant, singleNode("n1"))
	if err := repo.SaveObservationPointMapping(ctx, intruder, routeMapping("cam-1", "e2", 0.5)); !errors.Is(err, tenantctx.ErrMismatch) {
		t.Errorf("SaveObservationPointMapping() by another tenant error = %v, want %v", err, tenantctx.ErrMismatch)
	}

	got, err := repo.FindObservationPointMappings(ctx, graphEvent)
	if err != nil {
		t.Fatalf("FindObservationPointMappings() error = %v", err)
	}

	if want := []domain.ObservationPointMapping{routeMapping("cam-1", "e1", 0.5)}; !reflect.DeepEqual(got, want) {
		t.Errorf("FindObservationPointMappings() = %+v, want %+v", got, want)
	}
}
