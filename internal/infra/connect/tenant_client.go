package connect

import (
	"context"
	"fmt"

	connectrpc "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	tenantv1 "github.com/pj-hoakari/tolo-tenant-management/gen/tolo/tenant/v1"
	"github.com/pj-hoakari/tolo-tenant-management/gen/tolo/tenant/v1/tenantv1connect"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

type TenantClient struct {
	client tenantv1connect.TenantServiceClient
}

func NewTenantClient(httpClient connecthttp.HTTPClient, baseURL string) *TenantClient {
	return &TenantClient{
		client: tenantv1connect.NewTenantServiceClient(
			connectrpc.NewClient(connecthttp.NewTransport(httpClient, baseURL), forwardAuthorization()),
		),
	}
}

func (c *TenantClient) FindEvent(ctx context.Context, eventPublicID string) (domain.Event, error) {
	res, err := c.client.GetEvent(ctx, &tenantv1.GetEventRequest{EventId: eventPublicID})
	if connectrpc.CodeOf(err) == connectrpc.CodeNotFound {
		return domain.Event{}, application.ErrEventNotFound
	}

	if err != nil {
		return domain.Event{}, fmt.Errorf("call GetEvent: %w", err)
	}

	event := res.GetEvent()

	return domain.NewEvent(
		event.GetEventId(),
		event.GetTenantId(),
		event.GetStatus() == tenantv1.EventStatus_EVENT_STATUS_ARCHIVED,
	), nil
}
