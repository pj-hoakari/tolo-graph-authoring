// Package application contains the use cases of the graph authoring context:
// saving the draft of a graph, publishing it as a revision, and
// supplying the current revision to other contexts.
package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

type MapObservationPointInput struct {
	EventPublicID string
	Mapping       domain.ObservationPointMapping
}

type MapObservationPointUseCase interface {
	MapObservationPoint(context.Context, MapObservationPointInput) (domain.ObservationPointMapping, error)
}

type GetObservationPointMappingsInput struct {
	EventPublicID string
}

type ObservationPointMappings struct {
	RevisionID string
	Mappings   []domain.ObservationPointMapping
}

type GetObservationPointMappingsUseCase interface {
	GetObservationPointMappings(context.Context, GetObservationPointMappingsInput) (ObservationPointMappings, error)
}

type GetGraphInput struct {
	EventPublicID string
}

type SavedGraph struct {
	Graph      domain.Graph
	Placements domain.Placements
}

type GetGraphUseCase interface {
	GetGraph(context.Context, GetGraphInput) (SavedGraph, error)
}

type GraphSupplyUseCases interface {
	GetCurrentRevisionUseCase
	GetObservationPointMappingsUseCase
}

type GraphUseCases interface {
	SaveGraphUseCase
	MapObservationPointUseCase
	PublishRevisionUseCase
	GetGraphUseCase
	GraphSupplyUseCases
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
			if err == nil {
				err = s.verifyPlacementsKept(ctx, graph)
			}
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

func (s *GraphService) GetGraph(ctx context.Context, input GetGraphInput) (SavedGraph, error) {
	if input.EventPublicID == "" {
		return SavedGraph{}, ErrEventIDRequired
	}

	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return SavedGraph{}, tenantctx.ErrMissing
	}

	if err := tenantctx.EnsureEvent(ctx, input.EventPublicID); err != nil {
		return SavedGraph{}, err
	}

	graph, err := s.graphs.FindByEventPublicID(ctx, tenantPublicID, input.EventPublicID)
	if err != nil {
		return SavedGraph{}, err
	}

	if err := tenantctx.VerifyOwnership(ctx, graph.TenantPublicID()); err != nil {
		return SavedGraph{}, err
	}

	placements, err := s.graphs.FindPlacements(ctx, graph)
	if err != nil {
		return SavedGraph{}, err
	}

	return SavedGraph{Graph: graph, Placements: placements}, nil
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

func (s *GraphService) MapObservationPoint(ctx context.Context, input MapObservationPointInput) (domain.ObservationPointMapping, error) {
	err := s.editGraph(ctx, input.EventPublicID, func(ctx context.Context, graph domain.Graph) error {
		if err := graph.VerifyMapping(input.Mapping); err != nil {
			return err
		}

		placements, err := s.graphs.FindPlacements(ctx, graph)
		if err != nil {
			return err
		}

		if err := placements.VerifyMappable(input.Mapping.Anchor); err != nil {
			return err
		}

		return s.graphs.SaveObservationPointMapping(ctx, graph, input.Mapping)
	})
	if err != nil {
		return domain.ObservationPointMapping{}, err
	}

	return input.Mapping, nil
}

type AddQrLocationInput struct {
	EventPublicID string
	QrLocation    domain.QrLocation
}

func (s *GraphService) AddQrLocation(ctx context.Context, input AddQrLocationInput) (domain.QrLocation, error) {
	location := input.QrLocation
	location.ID = newQrLocationID()

	err := s.editGraph(ctx, input.EventPublicID, func(ctx context.Context, graph domain.Graph) error {
		if err := s.verifyQrLocation(ctx, graph, location); err != nil {
			return err
		}

		return s.graphs.AddQrLocation(ctx, graph, location)
	})
	if err != nil {
		return domain.QrLocation{}, err
	}

	return location, nil
}

type UpdateQrLocationInput struct {
	EventPublicID string
	QrLocation    domain.QrLocation
}

func (s *GraphService) UpdateQrLocation(ctx context.Context, input UpdateQrLocationInput) (domain.QrLocation, error) {
	err := s.editGraph(ctx, input.EventPublicID, func(ctx context.Context, graph domain.Graph) error {
		if err := s.verifyQrLocation(ctx, graph, input.QrLocation); err != nil {
			return err
		}

		return s.graphs.UpdateQrLocation(ctx, graph, input.QrLocation)
	})
	if err != nil {
		return domain.QrLocation{}, err
	}

	return input.QrLocation, nil
}

type RemoveQrLocationInput struct {
	EventPublicID string
	QrLocationID  string
}

func (s *GraphService) RemoveQrLocation(ctx context.Context, input RemoveQrLocationInput) error {
	return s.editGraph(ctx, input.EventPublicID, func(ctx context.Context, graph domain.Graph) error {
		return s.graphs.RemoveQrLocation(ctx, graph, input.QrLocationID)
	})
}

func (s *GraphService) verifyQrLocation(ctx context.Context, graph domain.Graph, location domain.QrLocation) error {
	if err := graph.VerifyQrLocation(location); err != nil {
		return err
	}

	placements, err := s.graphs.FindPlacements(ctx, graph)
	if err != nil {
		return err
	}

	return placements.VerifyQrPlaceable(location.Anchor)
}

func (s *GraphService) verifyPlacementsKept(ctx context.Context, graph domain.Graph) error {
	placements, err := s.graphs.FindPlacements(ctx, graph)
	if err != nil {
		return err
	}

	return graph.VerifyPlacements(placements)
}

func (s *GraphService) editGraph(ctx context.Context, eventPublicID string, edit func(context.Context, domain.Graph) error) error {
	if eventPublicID == "" {
		return ErrEventIDRequired
	}

	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return tenantctx.ErrMissing
	}

	if err := tenantctx.EnsureEvent(ctx, eventPublicID); err != nil {
		return err
	}

	if err := s.ensureEditableEvent(ctx, eventPublicID); err != nil {
		return err
	}

	return s.transactions.WithinTransaction(ctx, func(ctx context.Context) error {
		graph, err := s.graphs.FindByEventPublicIDForUpdate(ctx, tenantPublicID, eventPublicID)
		if err != nil {
			return err
		}

		if err := tenantctx.VerifyOwnership(ctx, graph.TenantPublicID()); err != nil {
			return err
		}

		return edit(ctx, graph)
	})
}

func (s *GraphService) GetObservationPointMappings(
	ctx context.Context, input GetObservationPointMappingsInput,
) (ObservationPointMappings, error) {
	if input.EventPublicID == "" {
		return ObservationPointMappings{}, ErrEventIDRequired
	}

	if err := tenantctx.VerifyEvent(ctx, input.EventPublicID); err != nil {
		return ObservationPointMappings{}, err
	}

	revision, err := s.graphs.FindCurrentRevision(ctx, input.EventPublicID)
	if err != nil {
		return ObservationPointMappings{}, err
	}

	if err := tenantctx.VerifyOwnership(ctx, revision.TenantPublicID); err != nil {
		return ObservationPointMappings{}, err
	}

	mappings, err := s.graphs.FindObservationPointMappings(ctx, input.EventPublicID)
	if err != nil {
		return ObservationPointMappings{}, err
	}

	return ObservationPointMappings{RevisionID: revision.RevisionID, Mappings: mappings}, nil
}

func newQrLocationID() string {
	id := make([]byte, 8)
	_, _ = rand.Read(id) //nolint:errcheck

	return hex.EncodeToString(id)
}
