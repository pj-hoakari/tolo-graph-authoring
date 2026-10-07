// Package repository defines the persistence contracts the graph authoring
// use cases depend on.
package repository

import (
	"context"
	"errors"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

var ErrGraphNotFound = errors.New("graph not found")

type GraphRepository interface {
	FindByEventPublicIDForUpdate(ctx context.Context, tenantPublicID, eventPublicID string) (domain.Graph, error)
	Save(ctx context.Context, graph domain.Graph) error
	Publish(ctx context.Context, graph domain.Graph) error
	FindCurrentRevision(ctx context.Context, eventPublicID string) (domain.PublishedRevision, error)
}
