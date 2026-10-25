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

func floorDocument() domain.GraphDocument {
	return domain.GraphDocument{
		Nodes: []domain.Node{
			{ID: "gate", Type: domain.NodeTypeTransitOnly, Labels: map[string]string{"ja": "門"}, GroupID: "floor-1"},
			{ID: "hall", Type: domain.NodeTypeGoal, Labels: map[string]string{"ja": "ホール", "en": "Hall"}, GroupID: "floor-1"},
			{ID: "outside", Type: domain.NodeTypeExternal, Labels: map[string]string{"ja": "外"}},
		},
		Groups: []domain.Group{
			{ID: "floor-1", Labels: map[string]string{"ja": "1階", "en": "1F"}},
		},
		Edges: []domain.Edge{
			{ID: "e1", SourceNodeID: "gate", TargetNodeID: "hall", Direction: domain.EdgeDirectionBothWays},
			{ID: "e-out", SourceNodeID: "outside", TargetNodeID: "gate", Direction: domain.EdgeDirectionOneWay},
		},
	}
}

func floorLayout() domain.GuideMapLayout {
	return domain.GuideMapLayout{
		Groups: []domain.GuideMapGroup{{GroupID: "floor-1"}},
		Objects: []domain.GuideMapObject{
			{
				ID: "p-hall", GroupID: "floor-1", Title: map[string]string{"ja": "大ホール", "en": "Main hall"}, Color: "indigo", PointID: "hall",
				Place: &domain.GuideMapPlace{Center: domain.GuideMapPosition{X: 10, Y: 20}, Shape: domain.GuideMapPlaceShapeRect, Width: 30, Height: 40},
			},
			{
				ID: "l-corridor", GroupID: "floor-1", Color: "slate", RouteID: "e1",
				Line: &domain.GuideMapLine{Points: []domain.GuideMapPosition{{X: 0, Y: 0}, {X: 5, Y: 5}}, Width: 4},
			},
			{
				ID: "t-floor", GroupID: "floor-1", Title: map[string]string{"ja": "本館1階"}, Color: "slate",
				Text: &domain.GuideMapText{Position: domain.GuideMapPosition{X: 1, Y: 2}, Size: 22},
			},
		},
		StartObjectID: "p-hall",
	}
}

func layoutGraph(t *testing.T) (*application.GraphService, domain.Graph) {
	t.Helper()

	repo := newTestGraphRepository(t)
	service := application.NewGraphService(repo, repo, callerTenantEvents{})
	document := floorDocument()

	graph, err := service.SaveGraph(editingContext(), application.SaveGraphInput{EventPublicID: graphEvent, Document: &document})
	if err != nil {
		t.Fatalf("SaveGraph() error = %v", err)
	}

	return service, graph
}

func supplyContext() context.Context {
	return internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{TokenUse: internaljwt.TokenUseService})
}

func TestGuideMapLayoutIsPublishedWithTheGraphAndSuppliedWithTheLabelsItRefersTo(t *testing.T) {
	service, graph := layoutGraph(t)
	ctx := editingContext()
	layout := floorLayout()

	meta, err := service.SaveGuideMapLayout(ctx, application.SaveGuideMapLayoutInput{EventPublicID: graphEvent, GuideMapLayout: &layout})
	if err != nil {
		t.Fatalf("SaveGuideMapLayout() error = %v", err)
	}

	if meta.DraftRevisionID == graph.DraftRevisionID() {
		t.Errorf("SaveGuideMapLayout() draft revision = %q, want it to differ from the draft without the layout", meta.DraftRevisionID)
	}

	draft, err := service.GetGraph(ctx, application.GetGraphInput{EventPublicID: graphEvent})
	if err != nil {
		t.Fatalf("GetGraph() error = %v", err)
	}

	if !reflect.DeepEqual(draft.Placements().GuideMap, &layout) || draft.DraftRevisionID() != meta.DraftRevisionID {
		t.Errorf("GetGraph() = layout %+v, draft revision %q, want the saved layout and %q", draft.Placements().GuideMap, draft.DraftRevisionID(), meta.DraftRevisionID)
	}

	published, err := service.PublishRevision(ctx, application.PublishRevisionInput{EventPublicID: graphEvent})
	if err != nil {
		t.Fatalf("PublishRevision() error = %v", err)
	}

	got, err := service.GetGuideMapLayout(supplyContext(), application.GetGuideMapLayoutInput{
		EventPublicID: graphEvent, RevisionID: published.RevisionID, TenantPublicID: ownerTenant,
	})
	if err != nil {
		t.Fatalf("GetGuideMapLayout() error = %v", err)
	}

	want := application.GuideMapLayout{
		RevisionID:     published.RevisionID,
		GuideMapLayout: &layout,
		Labels: domain.GuideMapLabels{
			Points: map[string]map[string]string{"hall": {"ja": "ホール", "en": "Hall"}},
			Groups: map[string]map[string]string{"floor-1": {"ja": "1階", "en": "1F"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetGuideMapLayout() = %+v, want %+v", got, want)
	}

	if _, err := service.GetGuideMapLayout(supplyContext(), application.GetGuideMapLayoutInput{
		EventPublicID: graphEvent, RevisionID: published.RevisionID, TenantPublicID: otherTenant,
	}); err == nil {
		t.Error("GetGuideMapLayout() for another tenant error = nil, want a tenant mismatch")
	}
}

func TestRevisionPublishedWithoutAGuideMapLayoutSuppliesNoLayout(t *testing.T) {
	service, _ := layoutGraph(t)

	published, err := service.PublishRevision(editingContext(), application.PublishRevisionInput{EventPublicID: graphEvent})
	if err != nil {
		t.Fatalf("PublishRevision() error = %v", err)
	}

	got, err := service.GetGuideMapLayout(supplyContext(), application.GetGuideMapLayoutInput{
		EventPublicID: graphEvent, RevisionID: published.RevisionID, TenantPublicID: ownerTenant,
	})
	if err != nil {
		t.Fatalf("GetGuideMapLayout() error = %v", err)
	}

	if got.GuideMapLayout != nil || len(got.Labels.Points) != 0 || len(got.Labels.Groups) != 0 {
		t.Errorf("GetGuideMapLayout() = %+v, want no layout and no labels", got)
	}
}

func TestGuideMapLayoutReferringToWhatIsNotAPointOrRouteOfTheGraphIsRejected(t *testing.T) {
	service, _ := layoutGraph(t)
	ctx := editingContext()

	for name, edit := range map[string]func(*domain.GuideMapLayout){
		"an external point":              func(l *domain.GuideMapLayout) { l.Objects[0].PointID = "outside" },
		"an edge to an external point":   func(l *domain.GuideMapLayout) { l.Objects[1].RouteID = "e-out" },
		"a point missing from the graph": func(l *domain.GuideMapLayout) { l.Objects[0].PointID = "stage" },
		"a group missing from the graph": func(l *domain.GuideMapLayout) { l.Groups = append(l.Groups, domain.GuideMapGroup{GroupID: "floor-9"}) },
	} {
		layout := floorLayout()
		edit(&layout)

		if _, err := service.SaveGuideMapLayout(ctx, application.SaveGuideMapLayoutInput{EventPublicID: graphEvent, GuideMapLayout: &layout}); !errors.Is(err, domain.ErrGuideMapLayoutTargetNotFound) {
			t.Errorf("SaveGuideMapLayout() referring to %s error = %v, want %v", name, err, domain.ErrGuideMapLayoutTargetNotFound)
		}
	}
}

func TestMalformedGuideMapLayoutIsRejected(t *testing.T) {
	service, _ := layoutGraph(t)
	ctx := editingContext()

	for name, edit := range map[string]func(*domain.GuideMapLayout){
		"an object on a group outside the layout": func(l *domain.GuideMapLayout) { l.Objects[0].GroupID = "floor-2" },
		"duplicate object IDs":                    func(l *domain.GuideMapLayout) { l.Objects[1].ID = "p-hall" },
		"an object without a kind":                func(l *domain.GuideMapLayout) { l.Objects[1].Line = nil },
		"a start that is not a place":             func(l *domain.GuideMapLayout) { l.StartObjectID = "t-floor" },
		"a connector without a link": func(l *domain.GuideMapLayout) {
			l.Objects[0].Place.Connector = &domain.GuideMapConnector{Type: domain.GuideMapConnectorTypeStairs, Direction: domain.GuideMapConnectorDirectionBoth}
		},
	} {
		layout := floorLayout()
		edit(&layout)

		if _, err := service.SaveGuideMapLayout(ctx, application.SaveGuideMapLayoutInput{EventPublicID: graphEvent, GuideMapLayout: &layout}); !errors.Is(err, domain.ErrInvalidGuideMapLayout) {
			t.Errorf("SaveGuideMapLayout() with %s error = %v, want %v", name, err, domain.ErrInvalidGuideMapLayout)
		}
	}
}

func TestSaveGraphRemovingWhatTheGuideMapLayoutRefersToIsRejected(t *testing.T) {
	service, _ := layoutGraph(t)
	ctx := editingContext()
	layout := floorLayout()

	if _, err := service.SaveGuideMapLayout(ctx, application.SaveGuideMapLayoutInput{EventPublicID: graphEvent, GuideMapLayout: &layout}); err != nil {
		t.Fatalf("SaveGuideMapLayout() error = %v", err)
	}

	withoutFloor := floorDocument()
	withoutFloor.Groups[0].ID = "floor-renamed"
	withoutFloor.Nodes[0].GroupID, withoutFloor.Nodes[1].GroupID = "floor-renamed", "floor-renamed"

	if _, err := service.SaveGraph(ctx, application.SaveGraphInput{EventPublicID: graphEvent, Document: &withoutFloor}); !errors.Is(err, domain.ErrGuideMapLayoutTargetNotFound) {
		t.Errorf("SaveGraph() without the group of the layout error = %v, want %v", err, domain.ErrGuideMapLayoutTargetNotFound)
	}
}

func TestSaveGuideMapLayoutBeforeTheGraphIsSavedIsNotFound(t *testing.T) {
	repo := newTestGraphRepository(t)
	service := application.NewGraphService(repo, repo, callerTenantEvents{})
	layout := floorLayout()

	if _, err := service.SaveGuideMapLayout(editingContext(), application.SaveGuideMapLayoutInput{EventPublicID: graphEvent, GuideMapLayout: &layout}); !errors.Is(err, repository.ErrGraphNotFound) {
		t.Errorf("SaveGuideMapLayout() without a graph error = %v, want %v", err, repository.ErrGraphNotFound)
	}
}
