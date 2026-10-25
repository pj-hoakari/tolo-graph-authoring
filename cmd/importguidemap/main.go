package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	connectrpc "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:8080", "base URL of the gateway or the service")
	eventID := flag.String("event", "", "event ID to save the guide map layout to")
	file := flag.String("file", "", "guide map layout JSON in the tolo-web fixture format")
	language := flag.String("lang", "ja", "language of titles and descriptions written as plain strings")

	flag.Parse()

	if err := run(*url, *eventID, *file, *language, os.Getenv("TOKEN")); err != nil {
		slog.Error("import guide map layout failed", "error", err)
		os.Exit(1)
	}
}

func run(url, eventID, file, language, token string) error {
	if eventID == "" || file == "" || token == "" {
		return errors.New("usage: TOKEN=<event_access token> importguidemap -event <event ID> -file <layout.json> [-url <base URL>] [-lang ja]")
	}

	encoded, err := os.ReadFile(file) //nolint:gosec // the operator names the file to import
	if err != nil {
		return err
	}

	layout, err := parseLayout(encoded, language)
	if err != nil {
		return err
	}

	client := graphv1connect.NewGraphAuthoringServiceClient(connectrpc.NewClient(connecthttp.NewTransport(http.DefaultClient, url)))
	ctx, info := connectrpc.NewClientContext(context.Background())
	info.RequestHeader().Set("Authorization", "Bearer "+token)

	meta, err := client.SaveGuideMapLayout(ctx, &graphv1.SaveGuideMapLayoutRequest{EventId: eventID, GuideMapLayout: layout})
	if err != nil {
		return fmt.Errorf("save guide map layout: %w", err)
	}

	fmt.Printf("saved: event %s, draft revision %s\n", meta.GetEventId(), meta.GetDraftRevisionId())

	return nil
}

type fixtureLayout struct {
	Groups []struct {
		GroupID string `json:"groupId"`
	} `json:"groups"`
	Objects []fixtureObject `json:"objects"`
	North   float64         `json:"north"`
	Scale   float64         `json:"scale"`
	StartID string          `json:"startId"`
}

type fixtureObject struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	GroupID     string          `json:"groupId"`
	Title       json.RawMessage `json:"title"`
	Description json.RawMessage `json:"description"`
	Color       string          `json:"color"`
	Closed      bool            `json:"closed"`
	PointID     string          `json:"pointId"`
	RouteID     string          `json:"routeId"`
	X           float64         `json:"x"`
	Y           float64         `json:"y"`
	W           float64         `json:"w"`
	H           float64         `json:"h"`
	Shape       string          `json:"shape"`
	Pin         bool            `json:"pin"`
	Connector   string          `json:"connector"`
	Link        string          `json:"link"`
	Direction   string          `json:"direction"`
	MarkW       *float64        `json:"markW"`
	MarkH       *float64        `json:"markH"`
	MarkerSize  *float64        `json:"markerSize"`
	Points      []struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"points"`
	Width float64 `json:"width"`
	Dash  bool    `json:"dash"`
	Size  float64 `json:"size"`
}

var placeShapes = map[string]graphv1.GuideMapPlaceShape{
	"pin":    graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_PIN,
	"rect":   graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_RECT,
	"circle": graphv1.GuideMapPlaceShape_GUIDE_MAP_PLACE_SHAPE_CIRCLE,
}

var connectorTypes = map[string]graphv1.GuideMapConnectorType{
	"stairs":    graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_STAIRS,
	"escalator": graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ESCALATOR,
	"elevator":  graphv1.GuideMapConnectorType_GUIDE_MAP_CONNECTOR_TYPE_ELEVATOR,
}

var connectorDirections = map[string]graphv1.GuideMapConnectorDirection{
	"":     graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_BOTH,
	"both": graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_BOTH,
	"up":   graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_UP,
	"down": graphv1.GuideMapConnectorDirection_GUIDE_MAP_CONNECTOR_DIRECTION_DOWN,
}

func parseLayout(encoded []byte, language string) (*graphv1.GuideMapLayout, error) {
	var fixture fixtureLayout
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		return nil, fmt.Errorf("parse guide map layout: %w", err)
	}

	layout := &graphv1.GuideMapLayout{
		Groups: make([]*graphv1.GuideMapGroup, 0, len(fixture.Groups)), Objects: make([]*graphv1.GuideMapObject, 0, len(fixture.Objects)),
		North: fixture.North, Scale: fixture.Scale, StartObjectId: fixture.StartID,
	}

	for _, group := range fixture.Groups {
		layout.Groups = append(layout.Groups, &graphv1.GuideMapGroup{GroupId: group.GroupID})
	}

	for _, object := range fixture.Objects {
		converted, err := object.toProto(language)
		if err != nil {
			return nil, fmt.Errorf("object %q: %w", object.ID, err)
		}

		layout.Objects = append(layout.Objects, converted)
	}

	return layout, nil
}

func (o fixtureObject) toProto(language string) (*graphv1.GuideMapObject, error) {
	title, err := localizedText(o.Title, language)
	if err != nil {
		return nil, err
	}

	description, err := localizedText(o.Description, language)
	if err != nil {
		return nil, err
	}

	object := &graphv1.GuideMapObject{
		ObjectId: o.ID, GroupId: o.GroupID, Title: title, Description: description,
		Color: o.Color, Closed: o.Closed, PointId: o.PointID, RouteId: o.RouteID, Kind: nil,
	}

	switch o.Kind {
	case "place":
		object.Kind = &graphv1.GuideMapObject_Place{Place: o.place()}
	case "line":
		object.Kind = &graphv1.GuideMapObject_Line{Line: &graphv1.GuideMapLine{Points: o.positions(), Width: o.Width, Dash: o.Dash}}
	case "area":
		object.Kind = &graphv1.GuideMapObject_Area{Area: &graphv1.GuideMapArea{Points: o.positions()}}
	case "text":
		object.Kind = &graphv1.GuideMapObject_Text{Text: &graphv1.GuideMapText{
			Position: &graphv1.GuideMapPosition{X: o.X, Y: o.Y}, Size: o.Size,
		}}
	default:
		return nil, fmt.Errorf("unknown kind %q", o.Kind)
	}

	return object, nil
}

func (o fixtureObject) place() *graphv1.GuideMapPlace {
	place := &graphv1.GuideMapPlace{
		Center: &graphv1.GuideMapPosition{X: o.X, Y: o.Y}, Shape: placeShapes[o.Shape],
		Width: o.W, Height: o.H, Pin: o.Pin, MarkWidth: o.MarkW, MarkHeight: o.MarkH, Connector: nil,
	}

	if place.MarkWidth == nil {
		place.MarkWidth = o.MarkerSize
	}

	if place.MarkHeight == nil {
		place.MarkHeight = o.MarkerSize
	}

	if o.Connector != "" {
		place.Connector = &graphv1.GuideMapConnector{
			Type: connectorTypes[o.Connector], Link: o.Link, Direction: connectorDirections[o.Direction],
		}
	}

	return place
}

func (o fixtureObject) positions() []*graphv1.GuideMapPosition {
	positions := make([]*graphv1.GuideMapPosition, 0, len(o.Points))
	for _, point := range o.Points {
		positions = append(positions, &graphv1.GuideMapPosition{X: point.X, Y: point.Y})
	}

	return positions
}

func localizedText(raw json.RawMessage, language string) (*graphv1.LocalizedText, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil //nolint:nilnil
	}

	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		if plain == "" {
			return nil, nil //nolint:nilnil
		}

		return &graphv1.LocalizedText{Texts: map[string]string{language: plain}}, nil
	}

	var texts map[string]string
	if err := json.Unmarshal(raw, &texts); err != nil {
		return nil, fmt.Errorf("text must be a string or an object of language codes: %w", err)
	}

	return &graphv1.LocalizedText{Texts: texts}, nil
}
