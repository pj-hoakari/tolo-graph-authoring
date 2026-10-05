package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

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
	graphs repository.GraphRepository
}

func NewGraphService(graphs repository.GraphRepository) *GraphService {
	return &GraphService{graphs: graphs}
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

	draftRevisionID, err := newRevisionID()
	if err != nil {
		return domain.VenueGraph{}, err
	}

	graph, err := s.graphs.FindByEventPublicID(ctx, input.EventPublicID)

	switch {
	case errors.Is(err, repository.ErrGraphNotFound):
		graph = domain.NewVenueGraph(tenantPublicID, input.EventPublicID, *input.Document, draftRevisionID)
	case err != nil:
		return domain.VenueGraph{}, err
	default:
		if err := tenantctx.VerifyOwnership(ctx, graph.TenantPublicID()); err != nil {
			return domain.VenueGraph{}, err
		}

		graph = graph.WithDraft(*input.Document, draftRevisionID)
	}

	if err := s.graphs.Save(ctx, graph); err != nil {
		return domain.VenueGraph{}, err
	}

	return graph, nil
}

func newRevisionID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate revision ID: %w", err)
	}

	return hex.EncodeToString(b), nil
}
