package connect

import (
	"context"
	"errors"

	connectrpc "connectrpc.com/connect"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
	kernelv1 "github.com/pj-hoakari/tolo-kernel-proto/gen/tolo/kernel/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	dbinfra "github.com/pj-hoakari/tolo-graph-authoring/internal/infra/db"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/tenantctx"
)

var nodeTypes = map[graphv1.NodeType]domain.NodeType{
	graphv1.NodeType_NODE_TYPE_GOAL:               domain.NodeTypeGoal,
	graphv1.NodeType_NODE_TYPE_GOAL_TRANSIT_MIXED: domain.NodeTypeGoalTransitMixed,
	graphv1.NodeType_NODE_TYPE_TRANSIT_ONLY:       domain.NodeTypeTransitOnly,
	graphv1.NodeType_NODE_TYPE_EXTERNAL:           domain.NodeTypeExternal,
}

var edgeDirections = map[graphv1.EdgeDirection]domain.EdgeDirection{
	graphv1.EdgeDirection_EDGE_DIRECTION_ONE_WAY:   domain.EdgeDirectionOneWay,
	graphv1.EdgeDirection_EDGE_DIRECTION_BOTH_WAYS: domain.EdgeDirectionBothWays,
}

var (
	protoNodeTypes      = inverted(nodeTypes)
	protoEdgeDirections = inverted(edgeDirections)
)

func inverted[K, V comparable](m map[K]V) map[V]K {
	inverse := make(map[V]K, len(m))
	for k, v := range m {
		inverse[v] = k
	}

	return inverse
}

type GraphService struct {
	graphv1connect.UnimplementedGraphAuthoringServiceHandler
	graphService application.GraphUseCases
}

func NewGraphService(graphService application.GraphUseCases) *GraphService {
	return &GraphService{
		UnimplementedGraphAuthoringServiceHandler: graphv1connect.UnimplementedGraphAuthoringServiceHandler{},
		graphService: graphService,
	}
}

func (s *GraphService) SaveGraph(ctx context.Context, req *connectrpc.Request[graphv1.SaveGraphRequest]) (*connectrpc.Response[graphv1.GraphMeta], error) {
	graph, err := s.graphService.SaveGraph(ctx, application.SaveGraphInput{
		EventPublicID: req.Msg.GetEventId(),
		Document:      graphDocumentFromProto(req.Msg.GetDocument()),
	})
	if err != nil {
		return nil, graphError(ctx, err)
	}

	return connectrpc.NewResponse(graphMeta(graph)), nil
}

func (s *GraphService) MapObservationPoint(
	ctx context.Context, req *connectrpc.Request[graphv1.MapObservationPointRequest],
) (*connectrpc.Response[graphv1.ObservationPointMapping], error) {
	mapping, err := s.graphService.MapObservationPoint(ctx, application.MapObservationPointInput{
		EventPublicID: req.Msg.GetEventId(),
		Mapping:       observationPointMappingFromProto(req.Msg.GetMapping()),
	})
	if err != nil {
		return nil, graphError(ctx, err)
	}

	return connectrpc.NewResponse(observationPointMappingToProto(mapping)), nil
}

func (s *GraphService) PublishRevision(ctx context.Context, req *connectrpc.Request[graphv1.PublishRevisionRequest]) (*connectrpc.Response[graphv1.GraphMeta], error) {
	graph, err := s.graphService.PublishRevision(ctx, application.PublishRevisionInput{EventPublicID: req.Msg.GetEventId()})
	if err != nil {
		return nil, graphError(ctx, err)
	}

	return connectrpc.NewResponse(graphMeta(graph)), nil
}

func (s *GraphService) GetGraph(ctx context.Context, req *connectrpc.Request[graphv1.GetGraphRequest]) (*connectrpc.Response[graphv1.GetGraphResponse], error) {
	saved, err := s.graphService.GetGraph(ctx, application.GetGraphInput{EventPublicID: req.Msg.GetEventId()})
	if err != nil {
		return nil, graphError(ctx, err)
	}

	mappings := make([]*graphv1.ObservationPointMapping, 0, len(saved.Placements.Mappings))
	for _, mapping := range saved.Placements.Mappings {
		mappings = append(mappings, observationPointMappingToProto(mapping))
	}

	locations := make([]*graphv1.QrLocation, 0, len(saved.Placements.QrLocations))
	for _, location := range saved.Placements.QrLocations {
		locations = append(locations, &graphv1.QrLocation{
			QrLocationId: location.ID,
			Name:         location.Name,
			Kind:         location.Kind,
			Anchor:       graphAnchorToProto(location.Anchor),
		})
	}

	return connectrpc.NewResponse(&graphv1.GetGraphResponse{
		Meta:                     graphMeta(saved.Graph),
		Document:                 graphDocumentToProto(saved.Graph.Draft()),
		ObservationPointMappings: mappings,
		QrLocations:              locations,
	}), nil
}

func graphMeta(graph domain.Graph) *graphv1.GraphMeta {
	return &graphv1.GraphMeta{
		EventId:         graph.EventPublicID(),
		RevisionId:      graph.RevisionID(),
		DraftRevisionId: graph.DraftRevisionID(),
	}
}

func graphError(ctx context.Context, err error) *connectrpc.Error {
	switch {
	case errors.Is(err, application.ErrEventIDRequired), errors.Is(err, application.ErrGraphDocumentRequired),
		errors.Is(err, domain.ErrInvalidGraphDocument), errors.Is(err, domain.ErrInvalidObservationPointMapping):
		return connectrpc.NewError(connectrpc.CodeInvalidArgument, err)
	case errors.Is(err, tenantctx.ErrMissing), errors.Is(err, tenantctx.ErrEventMissing):
		return connectrpc.NewError(connectrpc.CodeUnauthenticated, err)
	case errors.Is(err, tenantctx.ErrMismatch), errors.Is(err, tenantctx.ErrEventMismatch):
		return connectrpc.NewError(connectrpc.CodePermissionDenied, err)
	case errors.Is(err, application.ErrEventNotFound), errors.Is(err, application.ErrEventArchived),
		errors.Is(err, domain.ErrAnchorTargetNotFound), errors.Is(err, domain.ErrPlacementConflict):
		return connectrpc.NewError(connectrpc.CodeFailedPrecondition, err)
	case errors.Is(err, repository.ErrGraphNotFound):
		return connectrpc.NewError(connectrpc.CodeNotFound, err)
	case errors.Is(err, dbinfra.ErrTransactionAborted):
		return connectrpc.NewError(connectrpc.CodeAborted, err)
	default:
		return InternalError(ctx, err)
	}
}

func graphDocumentFromProto(document *graphv1.GraphDocument) *domain.GraphDocument {
	if document == nil {
		return nil
	}

	nodes := make([]domain.Node, 0, len(document.GetNodes()))
	for _, node := range document.GetNodes() {
		nodes = append(nodes, domain.Node{
			ID:      node.GetNodeId(),
			Type:    nodeTypes[node.GetNodeType()],
			Labels:  node.GetLabels(),
			GroupID: node.GetGroupId(),
			Layout:  layoutFromProto(node.GetLayout()),
		})
	}

	groups := make([]domain.Group, 0, len(document.GetGroups()))
	for _, group := range document.GetGroups() {
		groups = append(groups, domain.Group{
			ID:            group.GetGroupId(),
			Labels:        group.GetLabels(),
			ParentGroupID: group.GetParentGroupId(),
			MinWidth:      group.MinWidth,
			MinHeight:     group.MinHeight,
			Layout:        layoutFromProto(group.GetLayout()),
		})
	}

	edges := make([]domain.Edge, 0, len(document.GetEdges()))
	for _, edge := range document.GetEdges() {
		edges = append(edges, domain.Edge{
			ID:           edge.GetEdgeId(),
			SourceNodeID: edge.GetSourceNodeId(),
			TargetNodeID: edge.GetTargetNodeId(),
			Direction:    edgeDirections[edge.GetDirection()],
			Label:        edge.Label,
		})
	}

	return &domain.GraphDocument{Nodes: nodes, Groups: groups, Edges: edges}
}

func graphDocumentToProto(document domain.GraphDocument) *graphv1.GraphDocument {
	nodes := make([]*graphv1.GraphNode, 0, len(document.Nodes))
	for _, node := range document.Nodes {
		nodes = append(nodes, &graphv1.GraphNode{
			NodeId:   node.ID,
			NodeType: protoNodeTypes[node.Type],
			Labels:   node.Labels,
			GroupId:  node.GroupID,
			Layout:   layoutToProto(node.Layout),
		})
	}

	groups := make([]*graphv1.NodeGroup, 0, len(document.Groups))
	for _, group := range document.Groups {
		groups = append(groups, &graphv1.NodeGroup{
			GroupId:       group.ID,
			Labels:        group.Labels,
			Layout:        layoutToProto(group.Layout),
			ParentGroupId: group.ParentGroupID,
			MinWidth:      group.MinWidth,
			MinHeight:     group.MinHeight,
		})
	}

	edges := make([]*graphv1.GraphEdge, 0, len(document.Edges))
	for _, edge := range document.Edges {
		edges = append(edges, &graphv1.GraphEdge{
			EdgeId:       edge.ID,
			SourceNodeId: edge.SourceNodeID,
			TargetNodeId: edge.TargetNodeID,
			Direction:    protoEdgeDirections[edge.Direction],
			Label:        edge.Label,
		})
	}

	return &graphv1.GraphDocument{Nodes: nodes, Groups: groups, Edges: edges}
}

func layoutToProto(layout domain.Layout) *graphv1.Layout {
	return &graphv1.Layout{X: layout.X, Y: layout.Y, Width: layout.Width, Height: layout.Height}
}

func layoutFromProto(layout *graphv1.Layout) domain.Layout {
	if layout == nil {
		return domain.Layout{X: 0, Y: 0, Width: nil, Height: nil}
	}

	return domain.Layout{
		X:      layout.GetX(),
		Y:      layout.GetY(),
		Width:  layout.Width,
		Height: layout.Height,
	}
}

func observationPointMappingFromProto(mapping *graphv1.ObservationPointMapping) domain.ObservationPointMapping {
	anchor := mapping.GetAnchor()

	graphAnchor := domain.GraphAnchor{Kind: domain.AnchorKindUnspecified, ElementID: "", RoutePosition: nil}
	if anchor != nil {
		graphAnchor.RoutePosition = anchor.RoutePosition
	}

	switch target := anchor.GetTarget().(type) {
	case *kernelv1.GraphAnchor_PointId:
		graphAnchor.Kind, graphAnchor.ElementID = domain.AnchorKindPoint, target.PointId
	case *kernelv1.GraphAnchor_RouteId:
		graphAnchor.Kind, graphAnchor.ElementID = domain.AnchorKindRoute, target.RouteId
	}

	return domain.ObservationPointMapping{ObservationPointID: mapping.GetObservationPointId(), Anchor: graphAnchor}
}

func observationPointMappingToProto(mapping domain.ObservationPointMapping) *graphv1.ObservationPointMapping {
	return &graphv1.ObservationPointMapping{ObservationPointId: mapping.ObservationPointID, Anchor: graphAnchorToProto(mapping.Anchor)}
}

func graphAnchorToProto(graphAnchor domain.GraphAnchor) *kernelv1.GraphAnchor {
	anchor := &kernelv1.GraphAnchor{Target: nil, RoutePosition: graphAnchor.RoutePosition}

	switch graphAnchor.Kind {
	case domain.AnchorKindPoint:
		anchor.Target = &kernelv1.GraphAnchor_PointId{PointId: graphAnchor.ElementID}
	case domain.AnchorKindRoute:
		anchor.Target = &kernelv1.GraphAnchor_RouteId{RouteId: graphAnchor.ElementID}
	case domain.AnchorKindUnspecified:
	}

	return anchor
}
