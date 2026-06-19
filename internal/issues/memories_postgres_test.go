package issues

import (
	"context"
	"errors"
	"testing"

	"sortit/internal/domain"
)

func TestPostgresStoreMemoryCRUDAndSearch(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	memory := domain.Memory{
		ID:           "mem-pg-1",
		Title:        "Ridge is the default",
		Body:         "Issue search defaults to the ridge similarity model.",
		Kind:         domain.MemoryKindDecision,
		AnchorTags:   []string{"search", "backend"},
		AnchorRegion: "c-abc",
		TagScores: []domain.TagRelevance{
			{Tag: "search", Relevance: 0.92},
			{Tag: "backend", Relevance: 0.55},
		},
		Status:         domain.MemoryStatusActive,
		Source:         domain.MemorySourceSynthesized,
		SourceIssueIDs: []string{"issue-1", "issue-2"},
		Confidence:     0.8,
		CreatedBy:      "alice",
		Embedding:      []float64{1, 0, 0},
	}
	if err := store.UpsertMemory(ctx, memory); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.GetMemory(ctx, memory.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != memory.Title || got.Body != memory.Body {
		t.Fatalf("text round-trip mismatch: %+v", got)
	}
	if got.Kind != domain.MemoryKindDecision || got.Source != domain.MemorySourceSynthesized {
		t.Fatalf("enum round-trip mismatch: %+v", got)
	}
	if len(got.AnchorTags) != 2 || got.AnchorRegion != "c-abc" {
		t.Fatalf("anchor round-trip mismatch: %+v", got)
	}
	if len(got.TagScores) != 2 || got.TagScores[0].Tag != "search" {
		t.Fatalf("tag scores round-trip mismatch: %+v", got.TagScores)
	}
	if len(got.SourceIssueIDs) != 2 {
		t.Fatalf("source issue ids round-trip mismatch: %+v", got.SourceIssueIDs)
	}
	if len(got.Embedding) != 3 || got.Embedding[0] != 1 {
		t.Fatalf("embedding round-trip mismatch: %+v", got.Embedding)
	}
	if got.Confidence != 0.8 {
		t.Fatalf("confidence round-trip mismatch: %v", got.Confidence)
	}

	// Update preserves created_at/created_by (not in the UPDATE SET clause).
	created := got.CreatedAt
	got.Title = "Ridge default (updated)"
	got.CreatedBy = "intruder"
	if err := store.UpsertMemory(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, err := store.GetMemory(ctx, memory.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if updated.Title != "Ridge default (updated)" {
		t.Fatalf("title not updated: %q", updated.Title)
	}
	if !updated.CreatedAt.Equal(created) || updated.CreatedBy != "alice" {
		t.Fatalf("created_at/created_by should be preserved: %+v", updated)
	}

	// A superseded memory is excluded from similarity search.
	superseded := domain.Memory{
		ID:        "mem-pg-2",
		Body:      "old decision",
		Status:    domain.MemoryStatusSuperseded,
		Embedding: []float64{1, 0, 0},
	}
	if err := store.UpsertMemory(ctx, superseded); err != nil {
		t.Fatalf("upsert superseded: %v", err)
	}

	results, err := store.SearchMemories(ctx, []float64{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Memory.ID != memory.ID {
		t.Fatalf("expected only the active memory, got %+v", results)
	}
	if results[0].Similarity < 0.99 {
		t.Fatalf("expected near-1 cosine similarity, got %v", results[0].Similarity)
	}

	// Status filter on listing.
	active, err := store.ListMemories(ctx, MemoryListOptions{Status: domain.MemoryStatusActive})
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 || active[0].ID != memory.ID {
		t.Fatalf("expected one active memory, got %+v", active)
	}

	if err := store.DeleteMemory(ctx, memory.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetMemory(ctx, memory.ID); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("expected ErrMemoryNotFound after delete, got %v", err)
	}
}
