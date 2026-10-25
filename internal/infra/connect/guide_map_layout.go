package connect

import (
	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

var guideMapPlaceShapes = map[graphv1.GuideMapPlaceShape]domain.GuideMapPlaceShape{
	graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_PIN:    domain.GuideMapPlaceShapePin,
	graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_RECT:   domain.GuideMapPlaceShapeRect,
	graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_CIRCLE: domain.GuideMapPlaceShapeCircle,
}

var guideMapConnectorTypes = map[graphv1.GuideMapConnectorType]domain.GuideMapConnectorType{
	graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_STAIRS:    domain.GuideMapConnectorTypeStairs,
	graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ESCALATOR: domain.GuideMapConnectorTypeEscalator,
	graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ELEVATOR:  domain.GuideMapConnectorTypeElevator,
}

var guideMapConnectorDirections = map[graphv1.GuideMapConnectorDirection]domain.GuideMapConnectorDirection{
	graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_BOTH: domain.GuideMapConnectorDirectionBoth,
	graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_UP:   domain.GuideMapConnectorDirectionUp,
	graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_DOWN: domain.GuideMapConnectorDirectionDown,
}

var (
	protoGuideMapPlaceShapes         = inverted(guideMapPlaceShapes)
	protoGuideMapConnectorTypes      = inverted(guideMapConnectorTypes)
	protoGuideMapConnectorDirections = inverted(guideMapConnectorDirections)
)

func guideMapLayoutFromProto(layout *graphv1.GuideMapLayout) *domain.GuideMapLayout {
	if layout == nil {
		return nil
	}

	groups := make([]domain.GuideMapGroup, 0, len(layout.GetGroups()))
	for _, group := range layout.GetGroups() {
		groups = append(groups, domain.GuideMapGroup{GroupID: group.GetGroupId()})
	}

	objects := make([]domain.GuideMapObject, 0, len(layout.GetObjects()))
	for _, object := range layout.GetObjects() {
		objects = append(objects, guideMapObjectFromProto(object))
	}

	return &domain.GuideMapLayout{
		Groups:        groups,
		Objects:       objects,
		North:         layout.GetNorth(),
		Scale:         layout.GetScale(),
		StartObjectID: layout.GetStartObjectId(),
	}
}

func guideMapObjectFromProto(object *graphv1.GuideMapObject) domain.GuideMapObject {
	converted := domain.GuideMapObject{
		ID:          object.GetObjectId(),
		GroupID:     object.GetGroupId(),
		Title:       object.GetTitle().GetTexts(),
		Description: object.GetDescription().GetTexts(),
		Color:       object.GetColor(),
		Closed:      object.GetClosed(),
		PointID:     object.GetPointId(),
		RouteID:     object.GetRouteId(),
		Place:       nil,
		Line:        nil,
		Area:        nil,
		Text:        nil,
	}

	switch kind := object.GetKind().(type) {
	case *graphv1.GuideMapObject_Place:
		converted.Place = guideMapPlaceFromProto(kind.Place)
	case *graphv1.GuideMapObject_Line:
		converted.Line = &domain.GuideMapLine{
			Points: guideMapPositionsFromProto(kind.Line.GetPoints()),
			Width:  kind.Line.GetWidth(),
			Dash:   kind.Line.GetDash(),
		}
	case *graphv1.GuideMapObject_Area:
		converted.Area = &domain.GuideMapArea{Points: guideMapPositionsFromProto(kind.Area.GetPoints())}
	case *graphv1.GuideMapObject_Text:
		converted.Text = &domain.GuideMapText{
			Position: guideMapPositionFromProto(kind.Text.GetPosition()),
			Size:     kind.Text.GetSize(),
		}
	}

	return converted
}

func guideMapPlaceFromProto(place *graphv1.GuideMapPlace) *domain.GuideMapPlace {
	converted := &domain.GuideMapPlace{
		Center:     guideMapPositionFromProto(place.GetCenter()),
		Shape:      guideMapPlaceShapes[place.GetShape()],
		Width:      place.GetWidth(),
		Height:     place.GetHeight(),
		Pin:        place.GetPin(),
		Connector:  nil,
		MarkWidth:  place.MarkWidth,
		MarkHeight: place.MarkHeight,
	}

	if connector := place.GetConnector(); connector != nil {
		converted.Connector = &domain.GuideMapConnector{
			Type:      guideMapConnectorTypes[connector.GetType()],
			Link:      connector.GetLink(),
			Direction: guideMapConnectorDirections[connector.GetDirection()],
		}
	}

	return converted
}

func guideMapPositionFromProto(position *graphv1.GuideMapPosition) domain.GuideMapPosition {
	return domain.GuideMapPosition{X: position.GetX(), Y: position.GetY()}
}

func guideMapPositionsFromProto(positions []*graphv1.GuideMapPosition) []domain.GuideMapPosition {
	converted := make([]domain.GuideMapPosition, 0, len(positions))
	for _, position := range positions {
		converted = append(converted, guideMapPositionFromProto(position))
	}

	return converted
}

func guideMapLayoutToProto(layout *domain.GuideMapLayout) *graphv1.GuideMapLayout {
	if layout == nil {
		return nil
	}

	groups := make([]*graphv1.GuideMapGroup, 0, len(layout.Groups))
	for _, group := range layout.Groups {
		groups = append(groups, &graphv1.GuideMapGroup{GroupId: group.GroupID})
	}

	objects := make([]*graphv1.GuideMapObject, 0, len(layout.Objects))
	for _, object := range layout.Objects {
		objects = append(objects, guideMapObjectToProto(object))
	}

	return &graphv1.GuideMapLayout{
		Groups:        groups,
		Objects:       objects,
		North:         layout.North,
		Scale:         layout.Scale,
		StartObjectId: layout.StartObjectID,
	}
}

func guideMapObjectToProto(object domain.GuideMapObject) *graphv1.GuideMapObject {
	converted := &graphv1.GuideMapObject{
		ObjectId:    object.ID,
		GroupId:     object.GroupID,
		Title:       localizedTextToProto(object.Title),
		Description: localizedTextToProto(object.Description),
		Color:       object.Color,
		Closed:      object.Closed,
		PointId:     object.PointID,
		RouteId:     object.RouteID,
		Kind:        nil,
	}

	switch {
	case object.Place != nil:
		converted.Kind = &graphv1.GuideMapObject_Place{Place: guideMapPlaceToProto(*object.Place)}
	case object.Line != nil:
		converted.Kind = &graphv1.GuideMapObject_Line{Line: &graphv1.GuideMapLine{
			Points: guideMapPositionsToProto(object.Line.Points),
			Width:  object.Line.Width,
			Dash:   object.Line.Dash,
		}}
	case object.Area != nil:
		converted.Kind = &graphv1.GuideMapObject_Area{Area: &graphv1.GuideMapArea{Points: guideMapPositionsToProto(object.Area.Points)}}
	case object.Text != nil:
		converted.Kind = &graphv1.GuideMapObject_Text{Text: &graphv1.GuideMapText{
			Position: guideMapPositionToProto(object.Text.Position),
			Size:     object.Text.Size,
		}}
	}

	return converted
}

func guideMapPlaceToProto(place domain.GuideMapPlace) *graphv1.GuideMapPlace {
	converted := &graphv1.GuideMapPlace{
		Center:     guideMapPositionToProto(place.Center),
		Shape:      protoGuideMapPlaceShapes[place.Shape],
		Width:      place.Width,
		Height:     place.Height,
		Pin:        place.Pin,
		Connector:  nil,
		MarkWidth:  place.MarkWidth,
		MarkHeight: place.MarkHeight,
	}

	if connector := place.Connector; connector != nil {
		converted.Connector = &graphv1.GuideMapConnector{
			Type:      protoGuideMapConnectorTypes[connector.Type],
			Link:      connector.Link,
			Direction: protoGuideMapConnectorDirections[connector.Direction],
		}
	}

	return converted
}

func guideMapPositionToProto(position domain.GuideMapPosition) *graphv1.GuideMapPosition {
	return &graphv1.GuideMapPosition{X: position.X, Y: position.Y}
}

func guideMapPositionsToProto(positions []domain.GuideMapPosition) []*graphv1.GuideMapPosition {
	converted := make([]*graphv1.GuideMapPosition, 0, len(positions))
	for _, position := range positions {
		converted = append(converted, guideMapPositionToProto(position))
	}

	return converted
}

func localizedTextToProto(texts map[string]string) *graphv1.LocalizedText {
	if texts == nil {
		return nil
	}

	return &graphv1.LocalizedText{Texts: texts}
}

func localizedTextsToProto(texts map[string]map[string]string) map[string]*graphv1.LocalizedText {
	converted := make(map[string]*graphv1.LocalizedText, len(texts))
	for id, text := range texts {
		converted[id] = &graphv1.LocalizedText{Texts: text}
	}

	return converted
}
