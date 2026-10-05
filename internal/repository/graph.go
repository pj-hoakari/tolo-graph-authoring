package repository

import (
	"context"
	"errors"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

var ErrGraphNotFound = errors.New("graph not found")

type GraphRepository interface {
	FindByEventPublicID(ctx context.Context, eventPublicID string) (domain.VenueGraph, error)
	Save(ctx context.Context, graph domain.VenueGraph) error
}
