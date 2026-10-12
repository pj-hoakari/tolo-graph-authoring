package db

import (
	"context"
	"errors"
	"reflect"
	"testing"

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

func TestObservationPointMappedThroughServiceIsSuppliedOnceItsRevisionIsPublished(t *testing.T) {
	_, service, _ := placedGraph(t)
	editing := editingContext()

	before, err := service.PublishRevision(editing, application.PublishRevisionInput{EventPublicID: graphEvent})
	if err != nil {
		t.Fatalf("PublishRevision() error = %v", err)
	}

	pointMapping := domain.ObservationPointMapping{
		ObservationPointID: "cam-2",
		Anchor:             domain.GraphAnchor{Kind: domain.AnchorKindPoint, ElementID: "gate"},
	}

	var mapped application.MappedObservationPoint

	for _, mapping := range []domain.ObservationPointMapping{pointMapping, routeMapping("cam-1", "e1", 0.25), routeMapping("cam-1", "e1", 0.75)} {
		mapped, err = service.MapObservationPoint(editing, application.MapObservationPointInput{EventPublicID: graphEvent, Mapping: mapping})
		if err != nil {
			t.Fatalf("MapObservationPoint(%+v) error = %v", mapping, err)
		}
	}

	if mapped.Meta.RevisionID != before.RevisionID || mapped.Meta.DraftRevisionID == before.RevisionID {
		t.Errorf("MapObservationPoint() meta = %+v, want published revision %q and a new draft revision", mapped.Meta, before.RevisionID)
	}

	getMappings := func(revisionID string) application.ObservationPointMappings {
		t.Helper()

		got, err := service.GetObservationPointMappings(context.Background(), application.GetObservationPointMappingsInput{EventPublicID: graphEvent, RevisionID: revisionID})
		if err != nil {
			t.Fatalf("GetObservationPointMappings(%q) error = %v", revisionID, err)
		}

		return got
	}

	if got, want := getMappings(before.RevisionID), (application.ObservationPointMappings{RevisionID: before.RevisionID, Mappings: []domain.ObservationPointMapping{}}); !reflect.DeepEqual(got, want) {
		t.Errorf("GetObservationPointMappings() before publishing the mappings = %+v, want %+v", got, want)
	}

	after, err := service.PublishRevision(editing, application.PublishRevisionInput{EventPublicID: graphEvent})
	if err != nil {
		t.Fatalf("PublishRevision() error = %v", err)
	}

	if after.RevisionID != mapped.Meta.DraftRevisionID {
		t.Errorf("PublishRevision() revision = %q, want the draft revision of the last mapping %q", after.RevisionID, mapped.Meta.DraftRevisionID)
	}

	want := application.ObservationPointMappings{
		RevisionID: after.RevisionID,
		Mappings:   []domain.ObservationPointMapping{routeMapping("cam-1", "e1", 0.75), pointMapping},
	}
	if got := getMappings(after.RevisionID); !reflect.DeepEqual(got, want) {
		t.Errorf("GetObservationPointMappings() = %+v, want %+v", got, want)
	}
}

func TestPostgresGraphRepositorySaveObservationPointMappingKeepsOtherTenantOut(t *testing.T) {
	repo := newTestGraphRepository(t)
	ctx := context.Background()

	owned := saveGraph(t, repo, singleNode("n1")).Ref()
	if err := repo.SaveObservationPointMapping(ctx, owned, routeMapping("cam-1", "e1", 0.5)); err != nil {
		t.Fatalf("SaveObservationPointMapping() error = %v", err)
	}

	intruder := newGraph(t, otherTenant, singleNode("n1")).Ref()
	if err := repo.SaveObservationPointMapping(ctx, intruder, routeMapping("cam-1", "e2", 0.5)); !errors.Is(err, tenantctx.ErrMismatch) {
		t.Errorf("SaveObservationPointMapping() by another tenant error = %v, want %v", err, tenantctx.ErrMismatch)
	}

	if got, want := findPlacements(t, repo, owned).Mappings, []domain.ObservationPointMapping{routeMapping("cam-1", "e1", 0.5)}; !reflect.DeepEqual(got, want) {
		t.Errorf("FindPlacements() mappings = %+v, want %+v", got, want)
	}

	if got := findPlacements(t, repo, intruder).Mappings; len(got) != 0 {
		t.Errorf("FindPlacements() mappings for another tenant = %+v, want none", got)
	}
}
