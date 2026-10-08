package application

import (
	"context"
	"errors"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

var (
	ErrEventNotFound = errors.New("event not found")
	ErrEventArchived = errors.New("event is archived")
)

type EventDirectory interface {
	FindEvent(ctx context.Context, eventPublicID string) (domain.Event, error)
}
