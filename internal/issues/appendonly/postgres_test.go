package appendonly

import (
	"context"
	"sync"
	"testing"

	"sortit/internal/issues"
	"sortit/internal/testpostgres"
)

// appendonlyPostgresHarness is a single ParadeDB container shared across every test
// in this package, started once and reset between tests via snapshot/restore. This
// mirrors the pattern in internal/api and internal/issues: booting (and migrating) a
// container per test is the dominant cost, so one container amortizes it across the
// whole package. Harness.Acquire already serializes DB tests behind a mutex, so the
// tests run sequentially regardless.
var appendonlyPostgresHarness struct {
	once    sync.Once
	harness *testpostgres.Harness
	err     error
}

func appendonlyHarness(t *testing.T) *testpostgres.Harness {
	t.Helper()

	appendonlyPostgresHarness.once.Do(func() {
		appendonlyPostgresHarness.harness, appendonlyPostgresHarness.err = testpostgres.Start(context.Background(), "sortit_appendonly_test")
	})
	if appendonlyPostgresHarness.err != nil {
		t.Fatalf("start postgres test harness: %v", appendonlyPostgresHarness.err)
	}
	return appendonlyPostgresHarness.harness
}

func newAppendonlyTestStore(t *testing.T, ctx context.Context) *issues.PostgresStore {
	t.Helper()

	databaseURL := appendonlyHarness(t).Acquire(t, func(ctx context.Context, databaseURL string) error {
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
	return store
}
