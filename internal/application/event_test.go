package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/tenantctx"
	"go.uber.org/mock/gomock"
)

type fakeEvents map[string]domain.Event

func (f fakeEvents) FindEvent(_ context.Context, eventPublicID string) (domain.Event, error) {
	event, ok := f[eventPublicID]
	if !ok {
		return domain.Event{}, application.ErrEventNotFound
	}

	return event, nil
}

var activeEvent = fakeEvents{eventID: domain.NewEvent(eventID, tenantID, false)}

type unconsultedEvents struct{ t testing.TB }

func (u unconsultedEvents) FindEvent(_ context.Context, eventPublicID string) (domain.Event, error) {
	u.t.Errorf("FindEvent(%q) was called, want the event directory left unconsulted", eventPublicID)

	return domain.Event{}, application.ErrEventNotFound
}

func TestEditingRequiresAnEditableEventOfTheTenant(t *testing.T) {
	t.Parallel()

	edits := map[string]func(*application.GraphService) error{
		"SaveGraph": func(s *application.GraphService) error {
			_, err := s.SaveGraph(eventContext(tenantID, eventID), application.SaveGraphInput{EventPublicID: eventID, Document: document("n1")})

			return err
		},
		"PublishRevision": func(s *application.GraphService) error {
			_, err := s.PublishRevision(eventContext(tenantID, eventID), application.PublishRevisionInput{EventPublicID: eventID})

			return err
		},
	}

	events := []struct {
		name   string
		events fakeEvents
		want   error
	}{
		{"unknown event", fakeEvents{}, application.ErrEventNotFound},
		{"archived event", fakeEvents{eventID: domain.NewEvent(eventID, tenantID, true)}, application.ErrEventArchived},
		{"event of another tenant", fakeEvents{eventID: domain.NewEvent(eventID, "ffffffffffffffff", false)}, tenantctx.ErrMismatch},
	}

	for editName, edit := range edits {
		for _, tt := range events {
			t.Run(editName+"/"+tt.name, func(t *testing.T) {
				t.Parallel()

				service := application.NewGraphService(NewMockGraphRepository(gomock.NewController(t)), fakeTransactor{}, tt.events)

				if err := edit(service); !errors.Is(err, tt.want) {
					t.Errorf("%s() error = %v, want %v", editName, err, tt.want)
				}
			})
		}
	}
}
