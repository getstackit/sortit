package api

import (
	"context"
	"path/filepath"
	"testing"

	"splat/internal/issues"
)

func newSQLiteIssueStore(t *testing.T, seed []issues.Issue) *issues.SQLiteStore {
	t.Helper()

	store, err := issues.OpenSQLiteStore(context.Background(), filepath.Join(t.TempDir(), "issues.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close sqlite store: %v", err)
		}
	})

	if seed != nil {
		if err := store.Replace(context.Background(), seed); err != nil {
			t.Fatalf("seed sqlite store: %v", err)
		}
	}

	return store
}
