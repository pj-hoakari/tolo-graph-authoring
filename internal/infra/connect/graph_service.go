package connect

import (
	"context"
	"errors"

	connectrpc "connectrpc.com/connect"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1/graphv1connect"
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
	graphv1.NodeType_NODE_TYPE_BOUNDARY:           domain.NodeTypeBoundary,
}

var edgeDirections = map[graphv1.EdgeDirection]domain.EdgeDirection{
	graphv1.EdgeDirection_EDGE_DIRECTION_ONE_WAY:   domain.EdgeDirectionOneWay,
	graphv1.EdgeDirection_EDGE_DIRECTION_BOTH_WAYS: domain.EdgeDirectionBothWays,
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

func (s *GraphService) PublishRevision(ctx context.Context, req *connectrpc.Request[graphv1.PublishRevisionRequest]) (*connectrpc.Response[graphv1.GraphMeta], error) {
	graph, err := s.graphService.PublishRevision(ctx, application.PublishRevisionInput{EventPublicID: req.Msg.GetEventId()})
	if err != nil {
		return nil, graphError(ctx, err)
	}

	return connectrpc.NewResponse(graphMeta(graph)), nil
}

func graphMeta(graph domain.VenueGraph) *graphv1.GraphMeta {
	return &graphv1.GraphMeta{
		EventId:         graph.EventPublicID(),
		RevisionId:      graph.RevisionID(),
		DraftRevisionId: graph.DraftRevisionID(),
	}
}

func graphError(ctx context.Context, err error) *connectrpc.Error {
	switch {
	case errors.Is(err, application.ErrEventIDRequired), errors.Is(err, application.ErrGraphDocumentRequired),
		errors.Is(err, domain.ErrInvalidGraphDocument):
		return connectrpc.NewError(connectrpc.CodeInvalidArgument, err)
	case errors.Is(err, tenantctx.ErrMissing), errors.Is(err, tenantctx.ErrEventMissing):
		return connectrpc.NewError(connectrpc.CodeUnauthenticated, err)
	case errors.Is(err, tenantctx.ErrMismatch), errors.Is(err, tenantctx.ErrEventMismatch):
		return connectrpc.NewError(connectrpc.CodePermissionDenied, err)
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
			ID:     group.GetGroupId(),
			Labels: group.GetLabels(),
			Layout: layoutFromProto(group.GetLayout()),
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
