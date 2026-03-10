package api

import (
	"context"
	"os"
	"testing"

	"splat/internal/issues"
)

func newPostgresIssueStore(t *testing.T, seed []issues.Issue) *issues.PostgresStore {
	t.Helper()

	databaseURL := os.Getenv("SPLAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SPLAT_TEST_DATABASE_URL not set")
	}

	store, err := issues.OpenPostgresStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close postgres store: %v", err)
		}
	})

	// Clean all data for a fresh test
	if err := store.Replace(context.Background(), nil); err != nil {
		t.Fatalf("reset postgres store: %v", err)
	}

	if seed != nil {
		if err := store.Replace(context.Background(), seed); err != nil {
			t.Fatalf("seed postgres store: %v", err)
		}
	}

	return store
}
