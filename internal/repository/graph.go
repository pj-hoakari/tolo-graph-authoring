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
	ErrRevisionNotFound   = errors.New("revision not found")
)

type GraphRepository interface {
	FindByEventPublicID(ctx context.Context, tenantPublicID, eventPublicID string) (domain.Graph, error)
	FindByEventPublicIDForUpdate(ctx context.Context, tenantPublicID, eventPublicID string) (domain.Graph, error)
	FindDraftKernelForUpdate(ctx context.Context, tenantPublicID, eventPublicID string) (domain.DraftKernel, error)
	FindDraftRevisionForUpdate(ctx context.Context, tenantPublicID, eventPublicID string) (domain.DraftRevision, error)
	Save(ctx context.Context, graph domain.Graph) error
	SaveDraftRevisionID(ctx context.Context, graph domain.GraphRef, revisionID string) error
	Publish(ctx context.Context, graph domain.GraphRef, placements domain.Placements) error
	FindCurrentRevision(ctx context.Context, eventPublicID string) (domain.PublishedRevision, error)
	FindRevision(ctx context.Context, eventPublicID, revisionID string) (domain.PublishedRevision, error)
	SaveObservationPointMapping(ctx context.Context, graph domain.GraphRef, mapping domain.ObservationPointMapping) error
	FindPlacements(ctx context.Context, graph domain.GraphRef) (domain.Placements, error)
	AddQrLocation(ctx context.Context, graph domain.GraphRef, location domain.QrLocation) error
	UpdateQrLocation(ctx context.Context, graph domain.GraphRef, location domain.QrLocation) error
	RemoveQrLocation(ctx context.Context, graph domain.GraphRef, qrLocationID string) error
}
