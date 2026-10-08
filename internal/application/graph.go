// Package application contains the use cases of the graph authoring context:
// saving the draft of a graph, publishing it as a revision, and
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
	SaveGraph(context.Context, SaveGraphInput) (domain.Graph, error)
}

type PublishRevisionInput struct {
	EventPublicID string
}

type PublishRevisionUseCase interface {
	PublishRevision(context.Context, PublishRevisionInput) (domain.Graph, error)
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
	events       EventDirectory
}

func NewGraphService(graphs repository.GraphRepository, transactions repository.Transactor, events EventDirectory) *GraphService {
	return &GraphService{graphs: graphs, transactions: transactions, events: events}
}

func (s *GraphService) SaveGraph(ctx context.Context, input SaveGraphInput) (domain.Graph, error) {
	if input.EventPublicID == "" {
		return domain.Graph{}, ErrEventIDRequired
	}

	if input.Document == nil {
		return domain.Graph{}, ErrGraphDocumentRequired
	}

	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return domain.Graph{}, tenantctx.ErrMissing
	}

	if err := tenantctx.EnsureEvent(ctx, input.EventPublicID); err != nil {
		return domain.Graph{}, err
	}

	if err := s.ensureEditableEvent(ctx, input.EventPublicID); err != nil {
		return domain.Graph{}, err
	}

	var saved domain.Graph

	err := s.transactions.WithinTransaction(ctx, func(ctx context.Context) error {
		graph, err := s.graphs.FindByEventPublicIDForUpdate(ctx, tenantPublicID, input.EventPublicID)

		switch {
		case errors.Is(err, repository.ErrGraphNotFound):
			graph, err = domain.NewGraph(tenantPublicID, input.EventPublicID, *input.Document)
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
		return domain.Graph{}, err
	}

	return saved, nil
}

func (s *GraphService) PublishRevision(ctx context.Context, input PublishRevisionInput) (domain.Graph, error) {
	if input.EventPublicID == "" {
		return domain.Graph{}, ErrEventIDRequired
	}

	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return domain.Graph{}, tenantctx.ErrMissing
	}

	if err := tenantctx.EnsureEvent(ctx, input.EventPublicID); err != nil {
		return domain.Graph{}, err
	}

	if err := s.ensureEditableEvent(ctx, input.EventPublicID); err != nil {
		return domain.Graph{}, err
	}

	var published domain.Graph

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
		return domain.Graph{}, err
	}

	return published, nil
}

func (s *GraphService) ensureEditableEvent(ctx context.Context, eventPublicID string) error {
	event, err := s.events.FindEvent(ctx, eventPublicID)
	if err != nil {
		return err
	}

	if err := tenantctx.VerifyOwnership(ctx, event.TenantPublicID()); err != nil {
		return err
	}

	if event.Archived() {
		return ErrEventArchived
	}

	return nil
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
