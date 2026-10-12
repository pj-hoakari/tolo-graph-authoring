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
	ErrRevisionIDRequired    = errors.New("revision_id is required")
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
	PublishRevision(context.Context, PublishRevisionInput) (domain.DraftRevision, error)
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

type MappedObservationPoint struct {
	Mapping domain.ObservationPointMapping
	Meta    domain.GraphMeta
}

type MapObservationPointUseCase interface {
	MapObservationPoint(context.Context, MapObservationPointInput) (MappedObservationPoint, error)
}

type GetObservationPointMappingsInput struct {
	EventPublicID string
	RevisionID    string
}

type ObservationPointMappings struct {
	RevisionID string
	Mappings   []domain.ObservationPointMapping
}

type GetObservationPointMappingsUseCase interface {
	GetObservationPointMappings(context.Context, GetObservationPointMappingsInput) (ObservationPointMappings, error)
}

type AddQrLocationInput struct {
	EventPublicID string
	QrLocation    domain.QrLocation
}

type PlacedQrLocation struct {
	QrLocation domain.QrLocation
	Meta       domain.GraphMeta
}

type AddQrLocationUseCase interface {
	AddQrLocation(context.Context, AddQrLocationInput) (PlacedQrLocation, error)
}

type UpdateQrLocationInput struct {
	EventPublicID string
	QrLocation    domain.QrLocation
}

type UpdateQrLocationUseCase interface {
	UpdateQrLocation(context.Context, UpdateQrLocationInput) (PlacedQrLocation, error)
}

type RemoveQrLocationInput struct {
	EventPublicID string
	QrLocationID  string
}

type RemoveQrLocationUseCase interface {
	RemoveQrLocation(context.Context, RemoveQrLocationInput) (domain.GraphMeta, error)
}

type GetQrLocationsInput struct {
	EventPublicID string
	RevisionID    string
}

type QrLocations struct {
	RevisionID  string
	QrLocations []domain.QrLocation
}

type GetQrLocationsUseCase interface {
	GetQrLocations(context.Context, GetQrLocationsInput) (QrLocations, error)
}

type GetGraphInput struct {
	EventPublicID string
}

type GetGraphUseCase interface {
	GetGraph(context.Context, GetGraphInput) (domain.Graph, error)
}

type GraphSupplyUseCases interface {
	GetCurrentRevisionUseCase
	GetObservationPointMappingsUseCase
	GetQrLocationsUseCase
}

type GraphUseCases interface {
	SaveGraphUseCase
	MapObservationPointUseCase
	AddQrLocationUseCase
	UpdateQrLocationUseCase
	RemoveQrLocationUseCase
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

func (s *GraphService) PublishRevision(ctx context.Context, input PublishRevisionInput) (domain.DraftRevision, error) {
	if input.EventPublicID == "" {
		return domain.DraftRevision{}, ErrEventIDRequired
	}

	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return domain.DraftRevision{}, tenantctx.ErrMissing
	}

	if err := tenantctx.EnsureEvent(ctx, input.EventPublicID); err != nil {
		return domain.DraftRevision{}, err
	}

	if err := s.ensureEditableEvent(ctx, input.EventPublicID); err != nil {
		return domain.DraftRevision{}, err
	}

	var published domain.DraftRevision

	err := s.transactions.WithinTransaction(ctx, func(ctx context.Context) error {
		draft, err := s.graphs.FindDraftRevisionForUpdate(ctx, tenantPublicID, input.EventPublicID)
		if err != nil {
			return err
		}

		if err := tenantctx.VerifyOwnership(ctx, draft.TenantPublicID); err != nil {
			return err
		}

		placements, err := s.graphs.FindPlacements(ctx, draft.GraphRef)
		if err != nil {
			return err
		}

		if err := s.graphs.Publish(ctx, draft.GraphRef, placements); err != nil {
			return err
		}

		published = draft

		return nil
	})
	if err != nil {
		return domain.DraftRevision{}, err
	}

	return published, nil
}

func (s *GraphService) GetGraph(ctx context.Context, input GetGraphInput) (domain.Graph, error) {
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

	graph, err := s.graphs.FindByEventPublicID(ctx, tenantPublicID, input.EventPublicID)
	if err != nil {
		return domain.Graph{}, err
	}

	if err := tenantctx.VerifyOwnership(ctx, graph.TenantPublicID()); err != nil {
		return domain.Graph{}, err
	}

	return graph, nil
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
	revision, err := s.currentRevision(ctx, input.EventPublicID)
	if err != nil {
		return domain.KernelGraph{}, err
	}

	return revision.KernelGraph(), nil
}

func (s *GraphService) currentRevision(ctx context.Context, eventPublicID string) (domain.PublishedRevision, error) {
	return s.publishedRevision(ctx, eventPublicID, s.graphs.FindCurrentRevision)
}

func (s *GraphService) revision(ctx context.Context, eventPublicID, revisionID string) (domain.PublishedRevision, error) {
	if eventPublicID == "" {
		return domain.PublishedRevision{}, ErrEventIDRequired
	}

	if revisionID == "" {
		return domain.PublishedRevision{}, ErrRevisionIDRequired
	}

	return s.publishedRevision(ctx, eventPublicID, func(ctx context.Context, eventPublicID string) (domain.PublishedRevision, error) {
		return s.graphs.FindRevision(ctx, eventPublicID, revisionID)
	})
}

func (s *GraphService) publishedRevision(
	ctx context.Context, eventPublicID string, find func(context.Context, string) (domain.PublishedRevision, error),
) (domain.PublishedRevision, error) {
	if eventPublicID == "" {
		return domain.PublishedRevision{}, ErrEventIDRequired
	}

	if err := tenantctx.VerifyEvent(ctx, eventPublicID); err != nil {
		return domain.PublishedRevision{}, err
	}

	revision, err := find(ctx, eventPublicID)
	if err != nil {
		return domain.PublishedRevision{}, err
	}

	if err := tenantctx.VerifyOwnership(ctx, revision.TenantPublicID); err != nil {
		return domain.PublishedRevision{}, err
	}

	return revision, nil
}

func (s *GraphService) MapObservationPoint(ctx context.Context, input MapObservationPointInput) (MappedObservationPoint, error) {
	meta, err := s.editGraph(ctx, input.EventPublicID, func(ctx context.Context, draft domain.DraftKernel) error {
		if err := draft.VerifyMapping(input.Mapping); err != nil {
			return err
		}

		placements, err := s.graphs.FindPlacements(ctx, draft.GraphRef)
		if err != nil {
			return err
		}

		if err := placements.VerifyMappable(input.Mapping.Anchor); err != nil {
			return err
		}

		return s.graphs.SaveObservationPointMapping(ctx, draft.GraphRef, input.Mapping)
	})
	if err != nil {
		return MappedObservationPoint{}, err
	}

	return MappedObservationPoint{Mapping: input.Mapping, Meta: meta}, nil
}

func (s *GraphService) AddQrLocation(ctx context.Context, input AddQrLocationInput) (PlacedQrLocation, error) {
	location := input.QrLocation
	location.ID = newQrLocationID()

	meta, err := s.editGraph(ctx, input.EventPublicID, func(ctx context.Context, draft domain.DraftKernel) error {
		if err := s.verifyQrLocation(ctx, draft, location); err != nil {
			return err
		}

		return s.graphs.AddQrLocation(ctx, draft.GraphRef, location)
	})
	if err != nil {
		return PlacedQrLocation{}, err
	}

	return PlacedQrLocation{QrLocation: location, Meta: meta}, nil
}

func (s *GraphService) UpdateQrLocation(ctx context.Context, input UpdateQrLocationInput) (PlacedQrLocation, error) {
	meta, err := s.editGraph(ctx, input.EventPublicID, func(ctx context.Context, draft domain.DraftKernel) error {
		if err := s.verifyQrLocation(ctx, draft, input.QrLocation); err != nil {
			return err
		}

		return s.graphs.UpdateQrLocation(ctx, draft.GraphRef, input.QrLocation)
	})
	if err != nil {
		return PlacedQrLocation{}, err
	}

	return PlacedQrLocation{QrLocation: input.QrLocation, Meta: meta}, nil
}

func (s *GraphService) RemoveQrLocation(ctx context.Context, input RemoveQrLocationInput) (domain.GraphMeta, error) {
	return s.editGraph(ctx, input.EventPublicID, func(ctx context.Context, draft domain.DraftKernel) error {
		return s.graphs.RemoveQrLocation(ctx, draft.GraphRef, input.QrLocationID)
	})
}

func (s *GraphService) verifyQrLocation(ctx context.Context, draft domain.DraftKernel, location domain.QrLocation) error {
	if err := draft.VerifyQrLocation(location); err != nil {
		return err
	}

	placements, err := s.graphs.FindPlacements(ctx, draft.GraphRef)
	if err != nil {
		return err
	}

	return placements.VerifyQrPlaceable(location.Anchor)
}

func (s *GraphService) editGraph(
	ctx context.Context, eventPublicID string, edit func(context.Context, domain.DraftKernel) error,
) (domain.GraphMeta, error) {
	if eventPublicID == "" {
		return domain.GraphMeta{}, ErrEventIDRequired
	}

	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return domain.GraphMeta{}, tenantctx.ErrMissing
	}

	if err := tenantctx.EnsureEvent(ctx, eventPublicID); err != nil {
		return domain.GraphMeta{}, err
	}

	if err := s.ensureEditableEvent(ctx, eventPublicID); err != nil {
		return domain.GraphMeta{}, err
	}

	var meta domain.GraphMeta

	err := s.transactions.WithinTransaction(ctx, func(ctx context.Context) error {
		draft, err := s.graphs.FindDraftKernelForUpdate(ctx, tenantPublicID, eventPublicID)
		if err != nil {
			return err
		}

		if err := tenantctx.VerifyOwnership(ctx, draft.TenantPublicID); err != nil {
			return err
		}

		if err := edit(ctx, draft); err != nil {
			return err
		}

		placements, err := s.graphs.FindPlacements(ctx, draft.GraphRef)
		if err != nil {
			return err
		}

		draftRevisionID, err := domain.RevisionID(draft.DocumentHash, placements)
		if err != nil {
			return err
		}

		if err := s.graphs.SaveDraftRevisionID(ctx, draft.GraphRef, draftRevisionID); err != nil {
			return err
		}

		meta = domain.GraphMeta{EventPublicID: draft.EventPublicID, RevisionID: draft.RevisionID, DraftRevisionID: draftRevisionID}

		return nil
	})
	if err != nil {
		return domain.GraphMeta{}, err
	}

	return meta, nil
}

func (s *GraphService) GetObservationPointMappings(
	ctx context.Context, input GetObservationPointMappingsInput,
) (ObservationPointMappings, error) {
	revision, err := s.revision(ctx, input.EventPublicID, input.RevisionID)
	if err != nil {
		return ObservationPointMappings{}, err
	}

	return ObservationPointMappings{RevisionID: revision.RevisionID, Mappings: revision.Placements.Mappings}, nil
}

func (s *GraphService) GetQrLocations(ctx context.Context, input GetQrLocationsInput) (QrLocations, error) {
	revision, err := s.revision(ctx, input.EventPublicID, input.RevisionID)
	if err != nil {
		return QrLocations{}, err
	}

	return QrLocations{RevisionID: revision.RevisionID, QrLocations: revision.Placements.QrLocations}, nil
}

func newQrLocationID() string {
	id := make([]byte, 8)
	_, _ = rand.Read(id) //nolint:errcheck

	return hex.EncodeToString(id)
}
