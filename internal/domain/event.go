package domain

type Event struct {
	publicID       string
	tenantPublicID string
	archived       bool
}

func NewEvent(publicID, tenantPublicID string, archived bool) Event {
	return Event{publicID: publicID, tenantPublicID: tenantPublicID, archived: archived}
}

func (e Event) PublicID() string       { return e.publicID }
func (e Event) TenantPublicID() string { return e.tenantPublicID }
func (e Event) Archived() bool         { return e.archived }
