package connect

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	connectrpc "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"

	graphv1 "github.com/pj-hoakari/tolo-graph-authoring/gen/tolo/graph/v1"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/application"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	dbinfra "github.com/pj-hoakari/tolo-graph-authoring/internal/infra/db"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/tenantctx"
)

// errInternal is the only detail a client learns about an internal failure.
var errInternal = errors.New("internal error")

// InternalError reports a failure the client can do nothing about. The cause is
// written to the server log and replaced by a fixed message, so that no
// internal detail leaves the service. The log handler names the trace of the
// request context on the record, so an operator can find the failure in the
// trace it belongs to.
//
// A cancelled or timed-out request is the client going away rather than a
// server fault, so it keeps its own code and is not logged.
//
// It is exported for the other transports of this process, so that every
// service answers an internal failure the same way.
func InternalError(ctx context.Context, err error) *connectrpc.Error {
	if errors.Is(err, context.Canceled) {
		return connectrpc.NewError(connectrpc.CodeCanceled, "canceled").WithCause(err)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return connectrpc.NewError(connectrpc.CodeDeadlineExceeded, "deadline exceeded").WithCause(err)
	}

	slog.ErrorContext(ctx, "internal error", "error", err)

	return connectrpc.NewError(connectrpc.CodeInternal, errInternal.Error()).WithCause(errInternal) //nolint:forbidigo // the one place that builds internal errors
}

var clientErrors = []struct {
	target  error
	code    connectrpc.Code
	message string
}{
	{application.ErrEventIDRequired, connectrpc.CodeInvalidArgument, "event_id is required"},
	{application.ErrGraphDocumentRequired, connectrpc.CodeInvalidArgument, "document is required"},
	{domain.ErrInvalidGraphDocument, connectrpc.CodeInvalidArgument, "invalid graph document"},
	{domain.ErrInvalidObservationPointMapping, connectrpc.CodeInvalidArgument, "invalid observation point mapping"},
	{domain.ErrInvalidQrLocation, connectrpc.CodeInvalidArgument, "invalid QR location"},
	{application.ErrGuideMapLayoutRequired, connectrpc.CodeInvalidArgument, "guide_map_layout is required"},
	{domain.ErrInvalidGuideMapLayout, connectrpc.CodeInvalidArgument, "invalid guide map layout"},
	{application.ErrRevisionIDRequired, connectrpc.CodeInvalidArgument, "revision_id is required"},
	{application.ErrTenantIDRequired, connectrpc.CodeInvalidArgument, "tenant_id is required"},
	{tenantctx.ErrMissing, connectrpc.CodeUnauthenticated, "tenant ID is missing from the token"},
	{tenantctx.ErrEventMissing, connectrpc.CodeUnauthenticated, "event ID is missing from the token"},
	{tenantctx.ErrMismatch, connectrpc.CodePermissionDenied, "event belongs to another tenant"},
	{tenantctx.ErrEventMismatch, connectrpc.CodePermissionDenied, "event ID does not match the token"},
	{application.ErrEventNotFound, connectrpc.CodeFailedPrecondition, "event not found"},
	{application.ErrEventArchived, connectrpc.CodeFailedPrecondition, "event is archived"},
	{domain.ErrAnchorTargetNotFound, connectrpc.CodeFailedPrecondition, "anchor target is not in the graph"},
	{domain.ErrGuideMapLayoutTargetNotFound, connectrpc.CodeFailedPrecondition, "guide map layout target is not in the graph"},
	{domain.ErrPlacementConflict, connectrpc.CodeFailedPrecondition, "camera mapping and QR location cannot share a graph element"},
	{repository.ErrGraphNotFound, connectrpc.CodeNotFound, "graph not found"},
	{repository.ErrQrLocationNotFound, connectrpc.CodeNotFound, "QR location not found"},
	{repository.ErrRevisionNotFound, connectrpc.CodeNotFound, "revision not found"},
	{dbinfra.ErrTransactionAborted, connectrpc.CodeAborted, "transaction aborted; retry the request"},
}

func connectError(ctx context.Context, err error) *connectrpc.Error {
	var violations domain.GraphViolations
	if errors.As(err, &violations) {
		return graphViolationsError(violations, err)
	}

	for _, clientErr := range clientErrors {
		if errors.Is(err, clientErr.target) {
			return connectrpc.NewError(clientErr.code, clientErr.message).WithCause(err)
		}
	}

	return InternalError(ctx, err)
}

func graphViolationsError(violations domain.GraphViolations, err error) *connectrpc.Error {
	messages := make([]string, 0, len(violations))
	protoViolations := make([]*graphv1.GraphViolation, 0, len(violations))

	for _, violation := range violations {
		messages = append(messages, violation.Message())
		protoViolations = append(protoViolations, &graphv1.GraphViolation{
			Kind:        protoViolationKinds[violation.Kind],
			ElementKind: protoElementKinds[violation.ElementKind],
			ElementId:   violation.ElementID,
			RelatedIds:  violation.RelatedIDs,
		})
	}

	connectErr := connectrpc.NewError(connectrpc.CodeInvalidArgument, "invalid graph document: "+strings.Join(messages, "; ")).WithCause(err)

	detail, detailErr := connectproto.NewErrorDetail(&graphv1.GraphViolations{Violations: protoViolations})
	if detailErr != nil {
		return connectErr
	}

	return connectErr.WithDetail(detail)
}
