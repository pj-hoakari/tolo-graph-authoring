package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	connectrpc "connectrpc.com/connect/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
	dbinfra "github.com/pj-hoakari/tolo-graph-authoring/internal/infra/db"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/logging"
	"github.com/pj-hoakari/tolo-graph-authoring/internal/repository"
)

// TestInternalErrorHidesDetail keeps the cause of an internal failure out of
// the response and puts it in the server log instead. It cannot run in
// parallel: it reads the log back through the process-wide default logger.
func TestInternalErrorHidesDetail(t *testing.T) {
	logs := captureLog(t)
	ctx := withSampledSpan(t, context.Background())

	err := InternalError(ctx, errors.New("secret detail"))
	if got, want := err.Code(), connectrpc.CodeInternal; got != want {
		t.Fatalf("InternalError() code = %v, want %v", got, want)
	}

	if got, want := err.Message(), "internal error"; got != want {
		t.Errorf("Message() = %q, want %q", got, want)
	}

	if strings.Contains(err.Error(), "secret detail") {
		t.Errorf("error = %q, want it to omit the underlying failure", err)
	}

	entry := decodeLogEntry(t, logs)

	if got, want := entry["severity"], "ERROR"; got != want {
		t.Errorf("severity = %v, want %q", got, want)
	}

	if got, want := entry["message"], "internal error"; got != want {
		t.Errorf("message = %v, want %q", got, want)
	}

	if got, want := entry["error"], "secret detail"; got != want {
		t.Errorf("error = %v, want %q", got, want)
	}

	// The trace fields replace the trace_id the log line used to carry: the
	// handler reads them off the request context.
	for _, key := range []string{"logging.googleapis.com/trace", "logging.googleapis.com/spanId"} {
		if _, ok := entry[key]; !ok {
			t.Errorf("log entry = %v, want it to name %q", entry, key)
		}
	}
}

// TestCanceledRequestIsNotLogged pins a client that goes away to canceled: it
// is not a server fault, so nothing is logged. It shares the process-wide
// logger with TestInternalErrorHidesDetail and so cannot run in parallel
// either.
func TestCanceledRequestIsNotLogged(t *testing.T) {
	logs := captureLog(t)

	err := InternalError(context.Background(), context.Canceled)
	if got, want := err.Code(), connectrpc.CodeCanceled; got != want {
		t.Fatalf("InternalError() code = %v, want %v", got, want)
	}

	if logs.Len() != 0 {
		t.Errorf("log = %q, want nothing logged", logs.String())
	}
}

func TestConnectErrorAnswersWithFixedMessages(t *testing.T) {
	t.Parallel()

	_, violations := domain.NewGraph("tenant", "fedcba9876543210", domain.GraphDocument{
		Nodes: []domain.Node{{ID: "outside", Type: domain.NodeTypeExternal}},
	})

	tests := []struct {
		name        string
		err         error
		wantCode    connectrpc.Code
		wantMessage string
	}{
		{
			"wrapped sentinel",
			fmt.Errorf("find graph: %w", repository.ErrGraphNotFound),
			connectrpc.CodeNotFound,
			"graph not found",
		},
		{
			"sentinel wrapped with a request ID",
			fmt.Errorf("%w: observation point %q", domain.ErrAnchorTargetNotFound, "cam-1"),
			connectrpc.CodeFailedPrecondition,
			"anchor target is not in the graph",
		},
		{
			"aborted transaction joined to the database error",
			errors.Join(
				fmt.Errorf("save graph: %w", &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}),
				dbinfra.ErrTransactionAborted,
			),
			connectrpc.CodeAborted,
			"transaction aborted; retry the request",
		},
		{
			"wrapped cancellation",
			fmt.Errorf("find graph: %w", context.Canceled),
			connectrpc.CodeCanceled,
			"canceled",
		},
		{
			"wrapped deadline",
			fmt.Errorf("find graph: %w", context.DeadlineExceeded),
			connectrpc.CodeDeadlineExceeded,
			"deadline exceeded",
		},
		{
			"wrapped graph violations",
			fmt.Errorf("save graph: %w", violations),
			connectrpc.CodeInvalidArgument,
			`invalid graph document: external node "outside" has no edge`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := connectError(context.Background(), tt.err)
			if got := err.Code(); got != tt.wantCode {
				t.Errorf("connectError() code = %v, want %v", got, tt.wantCode)
			}

			if got := err.Message(); got != tt.wantMessage {
				t.Errorf("connectError() message = %q, want %q", got, tt.wantMessage)
			}
		})
	}
}

// captureLog installs the service's own handler over a buffer as the default
// logger for the duration of the test, so the assertions read the records in
// the shape production writes them. The default logger is process-wide, so a
// test that calls this one must not call t.Parallel().
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var logs bytes.Buffer

	previous := slog.Default()

	slog.SetDefault(logging.NewLogger(&logs, logging.Options{Level: slog.LevelDebug, AddSource: false, ProjectID: ""}))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &logs
}

// decodeLogEntry parses the single JSON record the captured log holds.
func decodeLogEntry(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()

	line := strings.TrimSpace(logs.String())
	if line == "" {
		t.Fatal("nothing was logged")
	}

	if strings.Contains(line, "\n") {
		t.Fatalf("log = %q, want a single record", line)
	}

	var entry map[string]any

	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("unmarshal %q: %v", line, err)
	}

	return entry
}

// withSampledSpan returns ctx carrying a valid, sampled span context, as the
// tracing interceptor leaves on a request it has handled.
func withSampledSpan(t *testing.T, ctx context.Context) context.Context {
	t.Helper()

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex() error = %v", err)
	}

	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex() error = %v", err)
	}

	return trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
}
