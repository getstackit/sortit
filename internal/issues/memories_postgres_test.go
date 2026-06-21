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

func TestPostgresStoreListMemoriesKindFilter(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	seed := []domain.Memory{
		{ID: "kf-decision", Body: "decision", Kind: domain.MemoryKindDecision, Status: domain.MemoryStatusActive},
		{ID: "kf-concept", Body: "concept", Kind: domain.MemoryKindConcept, SubjectTag: "ridge", Status: domain.MemoryStatusActive},
		{ID: "kf-overview", Body: "overview", Kind: domain.MemoryKindOverview, Status: domain.MemoryStatusActive},
	}
	for _, m := range seed {
		if err := store.UpsertMemory(ctx, m); err != nil {
			t.Fatalf("seed %s: %v", m.ID, err)
		}
	}

	concepts, err := store.ListMemories(ctx, MemoryListOptions{Kind: domain.MemoryKindConcept})
	if err != nil {
		t.Fatalf("list concepts: %v", err)
	}
	if len(concepts) != 1 || concepts[0].ID != "kf-concept" {
		t.Fatalf("expected only the concept, got %+v", concepts)
	}

	// Status and kind compose in the WHERE clause.
	activeConcepts, err := store.ListMemories(ctx, MemoryListOptions{Status: domain.MemoryStatusActive, Kind: domain.MemoryKindConcept})
	if err != nil {
		t.Fatalf("list active concepts: %v", err)
	}
	if len(activeConcepts) != 1 || activeConcepts[0].ID != "kf-concept" {
		t.Fatalf("expected only the active concept, got %+v", activeConcepts)
	}
}

func TestPostgresStoreOverviewSingleton(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	if _, err := store.GetActiveOverview(ctx); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("expected ErrMemoryNotFound with no overview, got %v", err)
	}

	first := domain.Memory{
		ID:     "ov-pg-1",
		Title:  "Sortit",
		Body:   "Sortit is an issue tracker built around a factor model over tag relevance.",
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
	if got.ID != "ov-pg-1" || got.Kind != domain.MemoryKindOverview {
		t.Fatalf("unexpected active overview: %+v", got)
	}

	// The partial unique index allows at most one active overview.
	second := domain.Memory{ID: "ov-pg-2", Body: "second", Kind: domain.MemoryKindOverview, Status: domain.MemoryStatusActive}
	if err := store.UpsertMemory(ctx, second); err == nil {
		t.Fatal("expected unique-index violation for a second active overview")
	}

	// Superseding the first frees the singleton for a new active overview.
	first.Status = domain.MemoryStatusSuperseded
	if err := store.UpsertMemory(ctx, first); err != nil {
		t.Fatalf("supersede first overview: %v", err)
	}
	if err := store.UpsertMemory(ctx, second); err != nil {
		t.Fatalf("expected the freed singleton to accept a new active overview: %v", err)
	}
	got, err = store.GetActiveOverview(ctx)
	if err != nil {
		t.Fatalf("get active overview after supersede: %v", err)
	}
	if got.ID != "ov-pg-2" {
		t.Fatalf("expected the second overview active, got %+v", got)
	}
}

func TestPostgresStoreConceptSubjectTag(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	// A concept memory round-trips its subject tag (normalized).
	concept := domain.Memory{
		ID:         "mem-concept-1",
		Title:      "Ridge regression",
		Body:       "Our search ranking uses diagonal-penalty ridge regression.",
		Kind:       domain.MemoryKindConcept,
		SubjectTag: "Ridge Regression",
		Status:     domain.MemoryStatusActive,
		Embedding:  []float64{1, 0, 0},
	}
	if err := store.UpsertMemory(ctx, concept); err != nil {
		t.Fatalf("upsert concept: %v", err)
	}
	got, err := store.GetMemory(ctx, concept.ID)
	if err != nil {
		t.Fatalf("get concept: %v", err)
	}
	if got.Kind != domain.MemoryKindConcept || got.SubjectTag != "ridge regression" {
		t.Fatalf("concept subject tag round-trip mismatch: %+v", got)
	}

	// A non-concept memory never carries a subject tag, even if one is supplied.
	decision := domain.Memory{
		ID:         "mem-decision-1",
		Body:       "some decision",
		Kind:       domain.MemoryKindDecision,
		SubjectTag: "ridge regression",
		Status:     domain.MemoryStatusActive,
	}
	if err := store.UpsertMemory(ctx, decision); err != nil {
		t.Fatalf("upsert decision: %v", err)
	}
	gotDecision, err := store.GetMemory(ctx, decision.ID)
	if err != nil {
		t.Fatalf("get decision: %v", err)
	}
	if gotDecision.SubjectTag != "" {
		t.Fatalf("non-concept must not carry a subject tag, got %q", gotDecision.SubjectTag)
	}

	// The partial unique index allows at most one active concept per subject tag.
	dup := domain.Memory{
		ID:         "mem-concept-dup",
		Body:       "duplicate concept for the same tag",
		Kind:       domain.MemoryKindConcept,
		SubjectTag: "ridge regression",
		Status:     domain.MemoryStatusActive,
	}
	if err := store.UpsertMemory(ctx, dup); err == nil {
		t.Fatal("expected unique-index violation for a second active concept on the same tag")
	}

	// Superseding the first frees the tag for a new active concept.
	got.Status = domain.MemoryStatusSuperseded
	if err := store.UpsertMemory(ctx, got); err != nil {
		t.Fatalf("supersede concept: %v", err)
	}
	if err := store.UpsertMemory(ctx, dup); err != nil {
		t.Fatalf("expected the freed tag to accept a new active concept: %v", err)
	}
}
