package api

import (
	"context"
	"sync"
	"testing"

	"splat/internal/issues"
	"splat/internal/testpostgres"
)

var apiPostgresHarness struct {
	once    sync.Once
	harness *testpostgres.Harness
	err     error
}

func newPostgresIssueStore(t *testing.T, seed []issues.Issue) *issues.PostgresStore {
	t.Helper()

	databaseURL := apiHarness(t).Acquire(t, func(ctx context.Context, databaseURL string) error {
		store, err := issues.OpenPostgresStore(ctx, databaseURL)
		if err != nil {
			return err
		}
		return store.Close()
	})

	store, err := issues.OpenPostgresStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close postgres store: %v", err)
		}
	})

	if seed != nil {
		if err := store.Replace(context.Background(), seed); err != nil {
			t.Fatalf("seed postgres store: %v", err)
		}
	}

	return store
}

func apiHarness(t *testing.T) *testpostgres.Harness {
	t.Helper()

	apiPostgresHarness.once.Do(func() {
		apiPostgresHarness.harness, apiPostgresHarness.err = testpostgres.Start(context.Background(), "splat_api_test")
	})
	if apiPostgresHarness.err != nil {
		t.Fatalf("start postgres test harness: %v", apiPostgresHarness.err)
	}
	return apiPostgresHarness.harness
}
