// Package application contains the use cases of the graph authoring context:
// saving the draft of a venue graph, publishing it as a revision, and
// supplying the current revision to other contexts.
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

type PublishRevisionInput struct {
	EventPublicID string
}

type PublishRevisionUseCase interface {
	PublishRevision(context.Context, PublishRevisionInput) (domain.VenueGraph, error)
}

type GetCurrentRevisionInput struct {
	EventPublicID string
}

type GetCurrentRevisionUseCase interface {
	GetCurrentRevision(context.Context, GetCurrentRevisionInput) (domain.KernelGraph, error)
}

type GraphUseCases interface {
	SaveGraphUseCase
	PublishRevisionUseCase
	GetCurrentRevisionUseCase
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

func (s *GraphService) PublishRevision(ctx context.Context, input PublishRevisionInput) (domain.VenueGraph, error) {
	if input.EventPublicID == "" {
		return domain.VenueGraph{}, ErrEventIDRequired
	}

	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return domain.VenueGraph{}, tenantctx.ErrMissing
	}

	if err := tenantctx.EnsureEvent(ctx, input.EventPublicID); err != nil {
		return domain.VenueGraph{}, err
	}

	var published domain.VenueGraph

	err := s.transactions.WithinTransaction(ctx, func(ctx context.Context) error {
		graph, err := s.graphs.FindByEventPublicIDForUpdate(ctx, tenantPublicID, input.EventPublicID)
		if err != nil {
			return err
		}

		if err := tenantctx.VerifyOwnership(ctx, graph.TenantPublicID()); err != nil {
			return err
		}

		graph = graph.Published()
		if err := s.graphs.Publish(ctx, graph); err != nil {
			return err
		}

		published = graph

		return nil
	})
	if err != nil {
		return domain.VenueGraph{}, err
	}

	return published, nil
}

func (s *GraphService) GetCurrentRevision(ctx context.Context, input GetCurrentRevisionInput) (domain.KernelGraph, error) {
	if input.EventPublicID == "" {
		return domain.KernelGraph{}, ErrEventIDRequired
	}

	if err := tenantctx.VerifyEvent(ctx, input.EventPublicID); err != nil {
		return domain.KernelGraph{}, err
	}

	revision, err := s.graphs.FindCurrentRevision(ctx, input.EventPublicID)
	if err != nil {
		return domain.KernelGraph{}, err
	}

	if err := tenantctx.VerifyOwnership(ctx, revision.TenantPublicID); err != nil {
		return domain.KernelGraph{}, err
	}

	return revision.KernelGraph(), nil
}
