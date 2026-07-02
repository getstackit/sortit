package testpostgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Pinned to match docker-compose.yml and .github/workflows/test.yml. A floating
// :latest tag silently kept tests on a stale cached image; an explicit tag keeps
// the test container in lockstep with local dev and CI.
const image = "paradedb/paradedb:0.24.0-pg18"

type Harness struct {
	container *postgres.PostgresContainer
	database  string
	url       string

	mu          sync.Mutex
	snapshotted bool
}

func Start(ctx context.Context, database string) (*Harness, error) {
	// Ryuk (the testcontainers reaper sidecar) is left ENABLED on purpose. Each
	// harness package terminates its container in TestMain, but that only runs on a
	// clean exit — an interrupted run (Ctrl-C, `go test -timeout` kill, IDE stop,
	// panic) would orphan the container with nothing to reap it. Ryuk is exactly that
	// backstop: it watches the session and force-removes the container when the test
	// process dies. Disabling it previously caused ParadeDB containers to pile up.
	container, err := postgres.Run(
		ctx,
		image,
		postgres.BasicWaitStrategies(),
		postgres.WithDatabase(database),
		postgres.WithUsername("sortit"),
		postgres.WithPassword("sortit"),
		postgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres test container: %w", err)
	}

	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("postgres test connection string: %w", err)
	}

	return &Harness{
		container: container,
		database:  database,
		url:       url,
	}, nil
}

func (h *Harness) Terminate(ctx context.Context) error {
	if h == nil || h.container == nil {
		return nil
	}
	return h.container.Terminate(ctx)
}

func (h *Harness) Acquire(t testing.TB, prepare func(context.Context, string) error) string {
	t.Helper()

	h.mu.Lock()
	t.Cleanup(h.mu.Unlock)

	ctx := context.Background()
	if !h.snapshotted {
		if err := prepare(ctx, h.url); err != nil {
			t.Fatalf("prepare postgres test database: %v", err)
		}
		if err := h.terminateDatabaseConnections(ctx, h.database); err != nil {
			t.Fatalf("terminate postgres test connections before snapshot: %v", err)
		}
		if err := h.container.Snapshot(ctx); err != nil {
			t.Fatalf("snapshot postgres test database: %v", err)
		}
		h.snapshotted = true
		return h.url
	}

	if err := h.terminateDatabaseConnections(ctx, h.database); err != nil {
		t.Fatalf("terminate postgres test connections before restore: %v", err)
	}
	if err := h.container.Restore(ctx); err != nil {
		t.Fatalf("restore postgres test database: %v", err)
	}
	return h.url
}

func (h *Harness) terminateDatabaseConnections(ctx context.Context, database string) error {
	adminURL, err := h.adminConnectionString()
	if err != nil {
		return err
	}

	db, err := sql.Open("pgx", adminURL)
	if err != nil {
		return fmt.Errorf("open postgres admin connection: %w", err)
	}
	defer db.Close() //nolint:errcheck

	if _, err := db.ExecContext(
		ctx,
		`SELECT pg_terminate_backend(pid)
		 FROM pg_stat_activity
		 WHERE datname = $1 AND pid <> pg_backend_pid()`,
		database,
	); err != nil {
		return fmt.Errorf("terminate active connections for %q: %w", database, err)
	}

	return nil
}

func (h *Harness) adminConnectionString() (string, error) {
	parsed, err := url.Parse(h.url)
	if err != nil {
		return "", fmt.Errorf("parse postgres test connection string: %w", err)
	}
	parsed.Path = "/postgres"
	return parsed.String(), nil
}
