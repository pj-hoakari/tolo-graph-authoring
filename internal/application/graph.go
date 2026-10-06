package application

import (
	"context"
	"errors"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/tenantctx"
)

var (
	ErrEventIDRequired       = errors.New("event_id is required")
	ErrGraphDocumentRequired = errors.New("document is required")
)

type SaveGraphInput struct {
	EventPublicID string
	Document      *domain.GraphDocument
}

type SaveGraphUseCase interface {
	SaveGraph(context.Context, SaveGraphInput) (domain.VenueGraph, error)
}

type GraphUseCases interface {
	SaveGraphUseCase
}

type GraphService struct {
	graphs       repository.GraphRepository
	transactions repository.Transactor
}

func NewGraphService(graphs repository.GraphRepository, transactions repository.Transactor) *GraphService {
	return &GraphService{graphs: graphs, transactions: transactions}
}

func (s *GraphService) SaveGraph(ctx context.Context, input SaveGraphInput) (domain.VenueGraph, error) {
	if input.EventPublicID == "" {
		return domain.VenueGraph{}, ErrEventIDRequired
	}

	if input.Document == nil {
		return domain.VenueGraph{}, ErrGraphDocumentRequired
	}

	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return domain.VenueGraph{}, tenantctx.ErrMissing
	}

	if err := tenantctx.EnsureEvent(ctx, input.EventPublicID); err != nil {
		return domain.VenueGraph{}, err
	}

	var saved domain.VenueGraph

	err := s.transactions.WithinTransaction(ctx, func(ctx context.Context) error {
		graph, err := s.graphs.FindByEventPublicIDForUpdate(ctx, tenantPublicID, input.EventPublicID)

		switch {
		case errors.Is(err, repository.ErrGraphNotFound):
			graph, err = domain.NewVenueGraph(tenantPublicID, input.EventPublicID, *input.Document)
		case err != nil:
			return err
		default:
			if err := tenantctx.VerifyOwnership(ctx, graph.TenantPublicID()); err != nil {
				return err
			}

			graph, err = graph.WithDraft(*input.Document)
		}

		if err != nil {
			return err
		}

		if err := s.graphs.Save(ctx, graph); err != nil {
			return err
		}

		saved = graph

		return nil
	})
	if err != nil {
		return domain.VenueGraph{}, err
	}

	return saved, nil
}
