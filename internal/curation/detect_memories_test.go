package curation

import (
	"context"
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

func TestDetectQuietMemoriesFlagsUnreinforced(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -60)

	// One issue sits right on top of the "reinforced" memory; nothing is near
	// the "quiet" one.
	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "i1", Raw: "active area", Status: issues.StatusOpen, CreatedAt: now, Embedding: []float64{1, 0, 0}},
	})
	mustUpsertMemory(t, store, domain.Memory{ID: "reinforced", Title: "near corpus", Status: domain.MemoryStatusActive, CreatedAt: old, Embedding: []float64{1, 0, 0}, Confidence: 1})
	mustUpsertMemory(t, store, domain.Memory{ID: "quiet", Title: "isolated", Status: domain.MemoryStatusActive, CreatedAt: old, Embedding: []float64{0, 0, 1}, Confidence: 1})

	det := NewMemoryDetector(store, store, nil)
	quiet, err := det.DetectQuietMemories(ctx, QuietParams{})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(quiet) != 1 || quiet[0].MemoryID != "quiet" {
		t.Fatalf("expected only 'quiet' flagged, got %+v", quiet)
	}
}

func TestDetectQuietMemoriesSparesYoungMemories(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Now().UTC()

	store := issues.NewInMemoryStore(nil)
	mustUpsertMemory(t, store, domain.Memory{ID: "fresh", Title: "just made", Status: domain.MemoryStatusActive, CreatedAt: now, Embedding: []float64{0, 1, 0}, Confidence: 1})

	det := NewMemoryDetector(store, store, nil)
	quiet, err := det.DetectQuietMemories(ctx, QuietParams{MinAgeDays: 30})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(quiet) != 0 {
		t.Fatalf("expected young memory spared, got %+v", quiet)
	}
}

func TestDetectRedundantMemoriesPairsAndPicksKeeper(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	store := issues.NewInMemoryStore(nil)
	mustUpsertMemory(t, store, domain.Memory{ID: "keep", Title: "high confidence", Status: domain.MemoryStatusActive, CreatedAt: base, Embedding: []float64{1, 0, 0}, Confidence: 0.9})
	mustUpsertMemory(t, store, domain.Memory{ID: "dupe", Title: "low confidence near-dup", Status: domain.MemoryStatusActive, CreatedAt: base.AddDate(0, 0, 1), Embedding: []float64{0.99, 0.01, 0}, Confidence: 0.4})
	mustUpsertMemory(t, store, domain.Memory{ID: "other", Title: "unrelated", Status: domain.MemoryStatusActive, CreatedAt: base, Embedding: []float64{0, 1, 0}, Confidence: 1})

	det := NewMemoryDetector(store, store, nil)
	pairs, err := det.DetectRedundantMemories(ctx, RedundantParams{MinSimilarity: 0.9})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected exactly 1 redundant pair, got %d: %+v", len(pairs), pairs)
	}
	p := pairs[0]
	if p.SuggestedKeep != "keep" || p.SuggestedSupersede != "dupe" {
		t.Fatalf("expected keep=keep supersede=dupe, got keep=%q supersede=%q", p.SuggestedKeep, p.SuggestedSupersede)
	}
	if p.Similarity < 0.9 {
		t.Fatalf("expected pair similarity >= 0.9, got %v", p.Similarity)
	}
}

func mustUpsertMemory(t *testing.T, store *issues.InMemoryStore, mem domain.Memory) {
	t.Helper()
	if err := store.UpsertMemory(context.Background(), mem); err != nil {
		t.Fatalf("upsert memory %s: %v", mem.ID, err)
	}
}
