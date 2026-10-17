package connect

import (
	"context"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	kernelv1 "github.com/pj-hoakari/tolo-kernel-proto/gen/tolo/kernel/v1"
)

var pointTypes = map[domain.PointType]kernelv1.PointType{
	domain.PointTypeGoal:             kernelv1.PointType_POINT_TYPE_GOAL,
	domain.PointTypeGoalTransitMixed: kernelv1.PointType_POINT_TYPE_GOAL_TRANSIT_MIXED,
	domain.PointTypeTransitOnly:      kernelv1.PointType_POINT_TYPE_TRANSIT_ONLY,
}

var directionAttributes = map[domain.DirectionAttribute]kernelv1.DirectionAttribute{
	domain.DirectionAttributeOneWay:   kernelv1.DirectionAttribute_DIRECTION_ATTRIBUTE_ONE_WAY,
	domain.DirectionAttributeBothWays: kernelv1.DirectionAttribute_DIRECTION_ATTRIBUTE_BOTH_WAYS,
}

var boundaryDirections = map[domain.BoundaryDirection]kernelv1.BoundaryDirection{
	domain.BoundaryDirectionEntry:        kernelv1.BoundaryDirection_BOUNDARY_DIRECTION_ENTRY,
	domain.BoundaryDirectionExit:         kernelv1.BoundaryDirection_BOUNDARY_DIRECTION_EXIT,
	domain.BoundaryDirectionEntryAndExit: kernelv1.BoundaryDirection_BOUNDARY_DIRECTION_ENTRY_AND_EXIT,
}

type GraphSupplyService struct {
	graphv1connect.UnimplementedGraphSupplyServiceHandler
	graphService application.GraphSupplyUseCases
}

func NewGraphSupplyService(graphService application.GraphSupplyUseCases) *GraphSupplyService {
	return &GraphSupplyService{
		UnimplementedGraphSupplyServiceHandler: graphv1connect.UnimplementedGraphSupplyServiceHandler{},
		graphService:                           graphService,
	}
}

func (s *GraphSupplyService) GetCurrentRevision(ctx context.Context, req *graphv1.GetCurrentRevisionRequest) (*kernelv1.Graph, error) {
	graph, err := s.graphService.GetCurrentRevision(ctx, application.GetCurrentRevisionInput{EventPublicID: req.GetEventId()})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return kernelGraphToProto(graph), nil
}

func (s *GraphSupplyService) GetObservationPointMappings(
	ctx context.Context, req *graphv1.GetMappingsRequest,
) (*graphv1.GetMappingsResponse, error) {
	result, err := s.graphService.GetObservationPointMappings(ctx, application.GetObservationPointMappingsInput{
		EventPublicID: req.GetEventId(),
		RevisionID:    req.GetRevisionId(),
	})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return &graphv1.GetMappingsResponse{RevisionId: result.RevisionID, Mappings: observationPointMappingsToProto(result.Mappings)}, nil
}

func (s *GraphSupplyService) GetQrLocations(
	ctx context.Context, req *graphv1.GetQrLocationsRequest,
) (*graphv1.GetQrLocationsResponse, error) {
	result, err := s.graphService.GetQrLocations(ctx, application.GetQrLocationsInput{
		EventPublicID:  req.GetEventId(),
		RevisionID:     req.GetRevisionId(),
		TenantPublicID: req.GetTenantId(),
	})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return &graphv1.GetQrLocationsResponse{RevisionId: result.RevisionID, QrLocations: qrLocationsToProto(result.QrLocations)}, nil
}

func kernelGraphToProto(graph domain.KernelGraph) *kernelv1.Graph {
	points := make([]*kernelv1.Point, 0, len(graph.Points))
	for _, point := range graph.Points {
		var boundary *kernelv1.Boundary
		if point.Boundary != nil {
			boundary = &kernelv1.Boundary{
				Direction: boundaryDirections[point.Boundary.Direction],
				Active:    point.Boundary.Active,
			}
		}

		points = append(points, &kernelv1.Point{
			PointId:  point.ID,
			Type:     pointTypes[point.Type],
			Boundary: boundary,
		})
	}

	routes := make([]*kernelv1.Route, 0, len(graph.Routes))
	for _, route := range graph.Routes {
		routes = append(routes, &kernelv1.Route{
			RouteId:      route.ID,
			FromPointId:  route.FromPointID,
			ToPointId:    route.ToPointID,
			Direction:    directionAttributes[route.Direction],
			CapacityHint: nil,
		})
	}

	return &kernelv1.Graph{
		EventId:    graph.EventPublicID,
		RevisionId: graph.RevisionID,
		Points:     points,
		Routes:     routes,
	}
}
