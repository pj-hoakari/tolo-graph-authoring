package connect

import (
	"testing"

	"google.golang.org/protobuf/proto"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
)

func TestGuideMapLayoutKeepsEveryFieldThroughTheDomain(t *testing.T) {
	t.Parallel()

	layout := &graphv1.GuideMapLayout{
		Groups: []*graphv1.GuideMapGroup{{GroupId: "g_1f000001"}, {GroupId: "g_2f000002"}},
		Objects: []*graphv1.GuideMapObject{
			{
				ObjectId: "c-stairs", GroupId: "g_1f000001", Color: "indigo", Closed: true, PointId: "n_4a110001",
				Title:       &graphv1.LocalizedText{Texts: map[string]string{"ja": "階段", "en": "Stairs"}},
				Description: &graphv1.LocalizedText{Texts: map[string]string{"ja": "北側"}},
				Kind: &graphv1.GuideMapObject_Place{Place: &graphv1.GuideMapPlace{
					Center: &graphv1.GuideMapPosition{X: 1.5, Y: 2.5}, Shape: graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_PIN,
					Width: 120, Height: 48, Pin: true, MarkWidth: proto.Float64(40), MarkHeight: proto.Float64(30),
					Connector: &graphv1.GuideMapConnector{
						Type:      graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ESCALATOR,
						Link:      "l-1",
						Direction: graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_UP,
					},
				}},
			},
			{
				ObjectId: "l-corridor", GroupId: "g_1f000001", Color: "slate", RouteId: "e_4a11c001",
				Kind: &graphv1.GuideMapObject_Line{Line: &graphv1.GuideMapLine{
					Points: []*graphv1.GuideMapPosition{{X: 0, Y: 0}, {X: 3, Y: 4}}, Width: 6, Dash: true,
				}},
			},
			{
				ObjectId: "a-hall", GroupId: "g_2f000002", Color: "rose",
				Kind: &graphv1.GuideMapObject_Area{Area: &graphv1.GuideMapArea{Points: []*graphv1.GuideMapPosition{{X: 1, Y: 1}}}},
			},
			{
				ObjectId: "t-title", GroupId: "g_2f000002", Color: "slate",
				Title: &graphv1.LocalizedText{Texts: map[string]string{"ja": "本館2階"}},
				Kind:  &graphv1.GuideMapObject_Text{Text: &graphv1.GuideMapText{Position: &graphv1.GuideMapPosition{X: 9, Y: 8}, Size: 22}},
			},
		},
		North:         90,
		Scale:         0.05,
		StartObjectId: "c-stairs",
	}

	if got := guideMapLayoutToProto(guideMapLayoutFromProto(layout)); !proto.Equal(got, layout) {
		t.Errorf("guideMapLayoutToProto(guideMapLayoutFromProto()) = %v, want %v", got, layout)
	}
}
