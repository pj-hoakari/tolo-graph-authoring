package main

import (
	"testing"

	"google.golang.org/protobuf/proto"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
)

func TestFixtureLayoutBecomesTheGuideMapLayoutOfTheProto(t *testing.T) {
	t.Parallel()

	fixture := `{
		"version": 1,
		"groups": [{"groupId": "g_1f000001"}],
		"north": 0, "scale": 0, "startId": "p-hall",
		"objects": [
			{"id": "p-hall", "kind": "place", "groupId": "g_1f000001", "color": "indigo", "x": 1, "y": 2, "shape": "rect", "w": 30, "h": 40,
			 "title": "ホール", "pointId": "n_4a110001"},
			{"id": "c-ev", "kind": "place", "groupId": "g_1f000001", "color": "indigo", "x": 3, "y": 4, "shape": "pin", "w": 0, "h": 0,
			 "title": {"ja": "エレベーター", "en": "Elevator"}, "connector": "elevator", "link": "l-1", "direction": "up", "markerSize": 40},
			{"id": "l-1", "kind": "line", "groupId": "g_1f000001", "color": "slate", "points": [{"x": 0, "y": 0}], "width": 4, "dash": true, "routeId": "e_4a11c001"},
			{"id": "a-1", "kind": "area", "groupId": "g_1f000001", "color": "rose", "points": [{"x": 5, "y": 6}], "closed": true},
			{"id": "t-1", "kind": "text", "groupId": "g_1f000001", "color": "slate", "x": 7, "y": 8, "size": 22, "title": "本館1F"}
		]
	}`

	got, err := parseLayout([]byte(fixture), "ja")
	if err != nil {
		t.Fatalf("parseLayout() error = %v", err)
	}

	want := &graphv1.GuideMapLayout{
		Groups:        []*graphv1.GuideMapGroup{{GroupId: "g_1f000001"}},
		StartObjectId: "p-hall",
		Objects: []*graphv1.GuideMapObject{
			{
				ObjectId: "p-hall", GroupId: "g_1f000001", Color: "indigo", PointId: "n_4a110001",
				Title: &graphv1.LocalizedText{Texts: map[string]string{"ja": "ホール"}},
				Kind: &graphv1.GuideMapObject_Place{Place: &graphv1.GuideMapPlace{
					Center: &graphv1.GuideMapPosition{X: 1, Y: 2}, Shape: graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_RECT, Width: 30, Height: 40,
				}},
			},
			{
				ObjectId: "c-ev", GroupId: "g_1f000001", Color: "indigo",
				Title: &graphv1.LocalizedText{Texts: map[string]string{"ja": "エレベーター", "en": "Elevator"}},
				Kind: &graphv1.GuideMapObject_Place{Place: &graphv1.GuideMapPlace{
					Center: &graphv1.GuideMapPosition{X: 3, Y: 4}, Shape: graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_PIN,
					MarkWidth: proto.Float64(40), MarkHeight: proto.Float64(40),
					Connector: &graphv1.GuideMapConnector{
						Type: graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ELEVATOR, Link: "l-1",
						Direction: graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_UP,
					},
				}},
			},
			{
				ObjectId: "l-1", GroupId: "g_1f000001", Color: "slate", RouteId: "e_4a11c001",
				Kind: &graphv1.GuideMapObject_Line{Line: &graphv1.GuideMapLine{Points: []*graphv1.GuideMapPosition{{X: 0, Y: 0}}, Width: 4, Dash: true}},
			},
			{
				ObjectId: "a-1", GroupId: "g_1f000001", Color: "rose", Closed: true,
				Kind: &graphv1.GuideMapObject_Area{Area: &graphv1.GuideMapArea{Points: []*graphv1.GuideMapPosition{{X: 5, Y: 6}}}},
			},
			{
				ObjectId: "t-1", GroupId: "g_1f000001", Color: "slate",
				Title: &graphv1.LocalizedText{Texts: map[string]string{"ja": "本館1F"}},
				Kind:  &graphv1.GuideMapObject_Text{Text: &graphv1.GuideMapText{Position: &graphv1.GuideMapPosition{X: 7, Y: 8}, Size: 22}},
			},
		},
	}

	if !proto.Equal(got, want) {
		t.Errorf("parseLayout() = %v, want %v", got, want)
	}
}
