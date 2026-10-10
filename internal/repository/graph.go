// Package repository defines the persistence contracts the graph authoring
// use cases depend on.
package repository

import (
	"context"
	"errors"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

var (
	ErrGraphNotFound      = errors.New("graph not found")
	ErrQrLocationNotFound = errors.New("QR location not found")
)

type GraphRepository interface {
	FindByEventPublicID(ctx context.Context, tenantPublicID, eventPublicID string) (domain.Graph, error)
	FindByEventPublicIDForUpdate(ctx context.Context, tenantPublicID, eventPublicID string) (domain.Graph, error)
	Save(ctx context.Context, graph domain.Graph) error
	Publish(ctx context.Context, graph domain.Graph) error
	FindCurrentRevision(ctx context.Context, eventPublicID string) (domain.PublishedRevision, error)
	SaveObservationPointMapping(ctx context.Context, graph domain.Graph, mapping domain.ObservationPointMapping) error
	FindObservationPointMappings(ctx context.Context, eventPublicID string) ([]domain.ObservationPointMapping, error)
	FindPlacements(ctx context.Context, graph domain.Graph) (domain.Placements, error)
	AddQrLocation(ctx context.Context, graph domain.Graph, location domain.QrLocation) error
	UpdateQrLocation(ctx context.Context, graph domain.Graph, location domain.QrLocation) error
	RemoveQrLocation(ctx context.Context, graph domain.Graph, qrLocationID string) error
}
