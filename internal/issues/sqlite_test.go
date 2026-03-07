package issues

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSQLiteStoreCreateListAndGet(t *testing.T) {
	store := newSQLiteTestStore(t)

	created, err := store.Create(context.Background(), CreateInput{
		Raw:       "  add sqlite storage  ",
		CreatedBy: "  Casey ",
		Tags:      []string{"backend", " backend ", ""},
		TagScores: []TagRelevance{{Tag: "backend", Relevance: 0.9}},
		Embedding: []float64{0.25, 0.5},
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	if created.ID != "issue-000001" {
		t.Fatalf("expected first generated ID, got %q", created.ID)
	}
	if created.Raw != "add sqlite storage" {
		t.Fatalf("expected trimmed raw value, got %q", created.Raw)
	}
	if created.CreatedBy != "Casey" {
		t.Fatalf("expected trimmed creator, got %q", created.CreatedBy)
	}
	if len(created.Tags) != 1 || created.Tags[0] != "backend" {
		t.Fatalf("expected sanitized tags, got %#v", created.Tags)
	}

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 stored issue, got %d", len(items))
	}
	if len(items[0].TagScores) != 1 {
		t.Fatalf("expected persisted tag scores, got %#v", items[0].TagScores)
	}
	if len(items[0].Embedding) != 2 {
		t.Fatalf("expected persisted embedding, got %#v", items[0].Embedding)
	}

	loaded, err := store.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get created issue: %v", err)
	}
	if loaded.ID != created.ID {
		t.Fatalf("expected %q, got %q", created.ID, loaded.ID)
	}
	if loaded.Raw != created.Raw {
		t.Fatalf("expected raw %q, got %q", created.Raw, loaded.Raw)
	}
}

func TestSQLiteStoreReplaceResetsSequenceFromLoadedItems(t *testing.T) {
	store := newSQLiteTestStore(t)

	if err := store.Replace(context.Background(), SeedIssues()); err != nil {
		t.Fatalf("replace issues with seeds: %v", err)
	}

	created, err := store.Create(context.Background(), CreateInput{
		Raw: "created after sample load",
	})
	if err != nil {
		t.Fatalf("create issue after replace: %v", err)
	}
	if created.ID != "issue-000007" {
		t.Fatalf("expected sequence to continue after seeded items, got %q", created.ID)
	}

	if err := store.Replace(context.Background(), nil); err != nil {
		t.Fatalf("clear store: %v", err)
	}

	resetCreated, err := store.Create(context.Background(), CreateInput{
		Raw: "created after reset",
	})
	if err != nil {
		t.Fatalf("create issue after reset: %v", err)
	}
	if resetCreated.ID != "issue-000001" {
		t.Fatalf("expected sequence reset after clearing store, got %q", resetCreated.ID)
	}
}

func newSQLiteTestStore(t *testing.T) *SQLiteStore {
	t.Helper()

	store, err := OpenSQLiteStore(context.Background(), filepath.Join(t.TempDir(), "issues.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close sqlite store: %v", err)
		}
	})
	return store
}
