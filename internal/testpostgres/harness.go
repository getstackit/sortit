package testpostgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const image = "paradedb/paradedb:latest"

type Harness struct {
	container *postgres.PostgresContainer
	database  string
	url       string

	mu          sync.Mutex
	snapshotted bool
}

func Start(ctx context.Context, database string) (*Harness, error) {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		if err := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true"); err != nil {
			return nil, fmt.Errorf("disable ryuk: %w", err)
		}
	}

	container, err := postgres.Run(
		ctx,
		image,
		postgres.BasicWaitStrategies(),
		postgres.WithDatabase(database),
		postgres.WithUsername("splat"),
		postgres.WithPassword("splat"),
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

func (h *Harness) ConnectionString() string {
	return h.url
}

func (h *Harness) DatabaseName() string {
	return h.database
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
