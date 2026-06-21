package issues

import (
	"context"
	"errors"
	"testing"

	"sortit/internal/domain"
)

func TestInMemoryStoreMemoryCRUD(t *testing.T) {
	store := NewInMemoryStore(nil)
	ctx := context.Background()

	memory := domain.Memory{
		ID:         "mem-1",
		Title:      "Ridge is the default",
		Body:       "Search defaults to the ridge similarity model.",
		Kind:       domain.MemoryKindDecision,
		AnchorTags: []string{"search"},
		Status:     domain.MemoryStatusActive,
		Source:     domain.MemorySourceManual,
		Embedding:  []float64{0.1, 0.2, 0.3},
	}
	if err := store.UpsertMemory(ctx, memory); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.GetMemory(ctx, "mem-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != memory.Title || got.Body != memory.Body {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not defaulted: %+v", got)
	}

	// Update preserves CreatedAt/CreatedBy.
	created := got.CreatedAt
	got.Title = "updated"
	got.CreatedBy = "someone-else"
	if err := store.UpsertMemory(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, err := store.GetMemory(ctx, "mem-1")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if updated.Title != "updated" {
		t.Fatalf("title not updated: %q", updated.Title)
	}
	if !updated.CreatedAt.Equal(created) {
		t.Fatalf("CreatedAt should be preserved, got %v want %v", updated.CreatedAt, created)
	}

	if err := store.DeleteMemory(ctx, "mem-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetMemory(ctx, "mem-1"); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("expected ErrMemoryNotFound, got %v", err)
	}
}

func TestInMemoryStoreListMemoriesFilterAndOrder(t *testing.T) {
	store := NewInMemoryStore(nil)
	ctx := context.Background()

	seed := []domain.Memory{
		{ID: "a", Body: "a", Status: domain.MemoryStatusActive},
		{ID: "b", Body: "b", Status: domain.MemoryStatusSuperseded},
		{ID: "c", Body: "c", Status: domain.MemoryStatusActive},
	}
	for _, m := range seed {
		if err := store.UpsertMemory(ctx, m); err != nil {
			t.Fatalf("seed %s: %v", m.ID, err)
		}
	}

	active, err := store.ListMemories(ctx, MemoryListOptions{Status: domain.MemoryStatusActive})
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active memories, got %d", len(active))
	}
	for _, m := range active {
		if m.Status != domain.MemoryStatusActive {
			t.Fatalf("unexpected status in active filter: %s", m.Status)
		}
	}

	all, err := store.ListMemories(ctx, MemoryListOptions{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(all))
	}

	limited, err := store.ListMemories(ctx, MemoryListOptions{Limit: 1})
	if err != nil {
		t.Fatalf("list limited: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1 memory with limit, got %d", len(limited))
	}
}

func TestInMemoryStoreOverviewSingleton(t *testing.T) {
	store := NewInMemoryStore(nil)
	ctx := context.Background()

	if _, err := store.GetActiveOverview(ctx); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("expected ErrMemoryNotFound with no overview, got %v", err)
	}

	first := domain.Memory{
		ID:     "ov-1",
		Body:   "Sortit is an issue tracker built around a factor model.",
		Kind:   domain.MemoryKindOverview,
		Status: domain.MemoryStatusActive,
	}
	if err := store.UpsertMemory(ctx, first); err != nil {
		t.Fatalf("upsert first overview: %v", err)
	}
	got, err := store.GetActiveOverview(ctx)
	if err != nil {
		t.Fatalf("get active overview: %v", err)
	}
	if got.ID != "ov-1" || got.Kind != domain.MemoryKindOverview {
		t.Fatalf("unexpected active overview: %+v", got)
	}

	// At most one active overview: a second active row is rejected.
	second := domain.Memory{ID: "ov-2", Body: "second", Kind: domain.MemoryKindOverview, Status: domain.MemoryStatusActive}
	if err := store.UpsertMemory(ctx, second); err == nil {
		t.Fatal("expected singleton violation for a second active overview")
	}

	// Superseding the first frees the singleton for the second.
	first.Status = domain.MemoryStatusSuperseded
	if err := store.UpsertMemory(ctx, first); err != nil {
		t.Fatalf("supersede first overview: %v", err)
	}
	if err := store.UpsertMemory(ctx, second); err != nil {
		t.Fatalf("expected freed singleton to accept the second overview: %v", err)
	}
	got, err = store.GetActiveOverview(ctx)
	if err != nil {
		t.Fatalf("get active overview after supersede: %v", err)
	}
	if got.ID != "ov-2" {
		t.Fatalf("expected the second overview active, got %+v", got)
	}
}

func TestInMemoryStoreSearchMemories(t *testing.T) {
	store := NewInMemoryStore(nil)
	ctx := context.Background()

	near := domain.Memory{ID: "near", Body: "near", Status: domain.MemoryStatusActive, Embedding: []float64{1, 0, 0}}
	far := domain.Memory{ID: "far", Body: "far", Status: domain.MemoryStatusActive, Embedding: []float64{0, 1, 0}}
	superseded := domain.Memory{ID: "old", Body: "old", Status: domain.MemoryStatusSuperseded, Embedding: []float64{1, 0, 0}}
	for _, m := range []domain.Memory{near, far, superseded} {
		if err := store.UpsertMemory(ctx, m); err != nil {
			t.Fatalf("seed %s: %v", m.ID, err)
		}
	}

	results, err := store.SearchMemories(ctx, []float64{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 active results (superseded excluded), got %d", len(results))
	}
	if results[0].Memory.ID != "near" {
		t.Fatalf("expected nearest memory first, got %q", results[0].Memory.ID)
	}
	if results[0].Similarity <= results[1].Similarity {
		t.Fatalf("results not ordered by similarity desc: %v", results)
	}
}
