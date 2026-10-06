package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

var testDB *sqlx.DB

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	container, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("tolo_graph_authoring"),
		postgres.WithUsername("tolo_graph_authoring"),
		postgres.WithPassword("tolo_graph_authoring"),
		postgres.WithInitScripts(migrationPaths()...),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start PostgreSQL test container: %v\n", err)
		os.Exit(1)
	}

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err == nil {
		// Open is the same instrumented entry point the server uses, so the
		// repository tests also cover the OpenTelemetry driver wrapper.
		testDB, err = Open(ctx, databaseURL)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "connect to PostgreSQL test container: %v\n", err)

		_ = container.Terminate(context.Background())

		os.Exit(1)
	}

	code := m.Run()
	_ = testDB.Close()
	_ = container.Terminate(context.Background())

	os.Exit(code)
}

// TestPostgresGraphRepositoryPublishJoinsTransaction proves the repository
// runs its statement through Executor: the write is rolled back with the
// surrounding transaction and only lands once that transaction commits.
func TestPostgresGraphRepositoryPublishJoinsTransaction(t *testing.T) {
	repo := newTestGraphRepository(t)
	graph := saveGraph(t, repo, singleNode("n1"))
	ctx := context.Background()

	errAbort := errors.New("abort")

	// The callback must return rather than call t.Fatal: a Goexit would skip
	// the rollback and leave the transaction holding its lock on graph_revisions.
	err := RunInTransaction(ctx, testDB, func(ctx context.Context) error {
		if err := repo.Publish(ctx, graph); err != nil {
			return fmt.Errorf("publish graph: %w", err)
		}

		return errAbort
	})
	if !errors.Is(err, errAbort) {
		t.Fatalf("RunInTransaction() error = %v, want %v", err, errAbort)
	}

	if count := countRevisions(t); count != 0 {
		t.Errorf("revisions count after rollback = %d, want 0", count)
	}

	if err := RunInTransaction(ctx, testDB, func(ctx context.Context) error {
		return repo.Publish(ctx, graph)
	}); err != nil {
		t.Fatalf("RunInTransaction() error = %v", err)
	}

	if count := countRevisions(t); count != 1 {
		t.Errorf("revisions count after commit = %d, want 1", count)
	}
}

func TestPostgresGraphRepositoryPublishNormalizesQueryText(t *testing.T) {
	// otel.SetTracerProvider mutates global state, so this test must not run
	// in parallel. otelsql keeps the delegating global provider it saw in
	// Open, which forwards to whatever is installed here.
	recorder := tracetest.NewSpanRecorder()
	previousProvider := otel.GetTracerProvider()

	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(previousProvider) })

	repo := newTestGraphRepository(t)
	publishGraph(t, repo, saveGraph(t, repo, singleNode("n1")))

	// The repository writes the statement as a multi-line raw string literal,
	// so this exact value also proves the newlines and tabs are gone.
	const wantQueryText = "INSERT INTO graph_revisions (event_public_id, tenant_public_id, revision_id, kernel, labels, layout) " +
		"SELECT event_public_id, tenant_public_id, revision_id, kernel, labels, layout FROM graph_drafts " +
		"WHERE event_public_id = $1 AND tenant_public_id = $2 " +
		"ON CONFLICT (event_public_id, revision_id) DO UPDATE SET last_published_at = EXCLUDED.last_published_at"

	var (
		insertSpan sdktrace.ReadOnlySpan
		spanNames  []string
	)

	for _, span := range recorder.Ended() {
		spanNames = append(spanNames, span.Name())

		if queryText, ok := spanAttribute(span, semconv.DBQueryTextKey); ok && queryText == wantQueryText {
			insertSpan = span
		}
	}

	if insertSpan == nil {
		t.Fatalf("span with %s = %q not found, recorded spans = %v", semconv.DBQueryTextKey, wantQueryText, spanNames)
	}

	system, ok := spanAttribute(insertSpan, semconv.DBSystemNameKey)
	if !ok {
		t.Fatalf("%s on span %q = missing, want %q", semconv.DBSystemNameKey, insertSpan.Name(), "postgresql")
	}

	if system != "postgresql" {
		t.Fatalf("%s on span %q = %q, want %q", semconv.DBSystemNameKey, insertSpan.Name(), system, "postgresql")
	}
}

func spanAttribute(span sdktrace.ReadOnlySpan, key attribute.Key) (string, bool) {
	for _, attr := range span.Attributes() {
		if attr.Key == key {
			return attr.Value.AsString(), true
		}
	}

	return "", false
}

func migrationPaths() []string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("locate test source file")
	}

	pattern := filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations", "*.up.sql")

	paths, err := filepath.Glob(pattern)
	if err != nil {
		panic(fmt.Sprintf("glob migration files %s: %v", pattern, err))
	}

	if len(paths) == 0 {
		panic(fmt.Sprintf("no migration files match %s", pattern))
	}

	slices.Sort(paths)

	return paths
}
