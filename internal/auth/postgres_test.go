package auth

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"sortit/internal/issues"
	"sortit/internal/testpostgres"
)

// authPostgresHarness is a single ParadeDB container shared across every DB test in
// this package, started once and reset between tests via snapshot/restore. Booting
// (and migrating) a container is the dominant cost, so one container amortizes it
// across the package — matching internal/api, internal/issues, and the appendonly
// package. Harness.Acquire serializes DB tests behind a mutex.
var authPostgresHarness struct {
	once    sync.Once
	harness *testpostgres.Harness
	err     error
}

func authHarness(t *testing.T) *testpostgres.Harness {
	t.Helper()

	authPostgresHarness.once.Do(func() {
		authPostgresHarness.harness, authPostgresHarness.err = testpostgres.Start(context.Background(), "sortit_auth_test")
	})
	if authPostgresHarness.err != nil {
		t.Fatalf("start postgres test harness: %v", authPostgresHarness.err)
	}
	return authPostgresHarness.harness
}

// newAuthTestDB returns a migrated database handle. Migrations live in the issues
// package (the auth tables share the issuesdb schema), so we open an issues store
// to run them and hand back the underlying *sql.DB for the auth Store under test.
func newAuthTestDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()

	databaseURL := authHarness(t).Acquire(t, func(ctx context.Context, databaseURL string) error {
		store, err := issues.OpenPostgresStore(ctx, databaseURL)
		if err != nil {
			return err
		}
		return store.Close()
	})

	store, err := issues.OpenPostgresStore(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close postgres store: %v", err)
		}
	})
	return store.DB()
}
