package memory

import (
	"context"
	"sync"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
)

type GraphRepository struct {
	mu     sync.Mutex
	graphs map[string]domain.VenueGraph
}

func NewGraphRepository() *GraphRepository {
	return &GraphRepository{mu: sync.Mutex{}, graphs: map[string]domain.VenueGraph{}}
}

func (r *GraphRepository) FindByEventPublicID(_ context.Context, eventPublicID string) (domain.VenueGraph, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	graph, ok := r.graphs[eventPublicID]
	if !ok {
		return domain.VenueGraph{}, repository.ErrGraphNotFound
	}

	return graph, nil
}

func (r *GraphRepository) Save(_ context.Context, graph domain.VenueGraph) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.graphs[graph.EventPublicID()] = graph

	return nil
}
