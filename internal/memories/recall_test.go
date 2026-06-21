package memories

import (
	"context"
	"errors"
	"testing"

	"sortit/internal/ai"
	"sortit/internal/domain"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
)

func seedMemory(t *testing.T, store issues.MemoryStore, id, body string, embedding []float64) {
	t.Helper()
	if err := store.UpsertMemory(context.Background(), domain.Memory{
		ID:        id,
		Body:      body,
		Status:    domain.MemoryStatusActive,
		Embedding: embedding,
	}); err != nil {
		t.Fatalf("seed memory %s: %v", id, err)
	}
}

func TestRecallMemoriesRanksBySimilarity(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	seedMemory(t, store, "mem-1", "ridge is the default", []float64{1, 0})
	seedMemory(t, store, "mem-2", "exports use the print pipeline", []float64{0, 1})

	// Query embeds parallel to mem-1, so it should rank first.
	enricher := &stubEnricher{embed: []float64{1, 0}}
	svc := NewService(store, enricher, nil)

	results, err := svc.RecallMemories(context.Background(), "how does search rank?", RecallOptions{})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "mem-1" {
		t.Fatalf("expected mem-1 first, got %s (%v)", results[0].ID, results)
	}
	if results[0].Similarity <= results[1].Similarity {
		t.Fatalf("expected descending similarity, got %v then %v", results[0].Similarity, results[1].Similarity)
	}
	// The query text is what gets embedded.
	if enricher.gotEmbed != "how does search rank?" {
		t.Fatalf("expected query embedded, got %q", enricher.gotEmbed)
	}
}

func TestRecallSurfacesSubjectConcepts(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	ctx := context.Background()

	// A high-cosine non-concept and a concept whose embedding is orthogonal to the
	// query (so cosine alone would never surface it).
	seedMemory(t, store, "mem-aligned", "aligned decision", []float64{1, 0})
	if err := store.UpsertMemory(ctx, domain.Memory{
		ID:         "mem-concept-ridge",
		Title:      "Ridge regression",
		Body:       "Our ranking uses diagonal-penalty ridge regression.",
		Kind:       domain.MemoryKindConcept,
		SubjectTag: "ridge",
		Status:     domain.MemoryStatusActive,
		Embedding:  []float64{0, 1},
	}); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	// The query analyzes to a dominant "ridge" tag and embeds parallel to the
	// aligned memory.
	enricher := &stubEnricher{
		embed: []float64{1, 0},
		result: issueenrichment.AnalyzeTextResult{
			Analyzed:  ai.AnalyzedIssue{Embedding: ai.EmbeddingResult{Vector: []float32{1, 0}}},
			TagScores: []issues.TagRelevance{{Tag: "ridge", Relevance: 0.9}},
		},
	}
	svc := NewService(store, enricher, nil)

	// Without the option, the orthogonal concept is not surfaced.
	plain, err := svc.RecallMemories(ctx, "how does ranking work?", RecallOptions{Limit: 1})
	if err != nil {
		t.Fatalf("plain recall: %v", err)
	}
	if len(plain) != 1 || plain[0].ID != "mem-aligned" {
		t.Fatalf("expected only the aligned memory, got %+v", plain)
	}

	// With the option, the dominant-tag concept leads even though its cosine is low.
	withConcepts, err := svc.RecallMemories(ctx, "how does ranking work?", RecallOptions{
		Limit:                  2,
		IncludeSubjectConcepts: true,
	})
	if err != nil {
		t.Fatalf("concept recall: %v", err)
	}
	if len(withConcepts) != 2 {
		t.Fatalf("expected 2 results, got %+v", withConcepts)
	}
	if withConcepts[0].ID != "mem-concept-ridge" {
		t.Fatalf("expected the ridge concept to lead, got %s", withConcepts[0].ID)
	}

	// The concept is force-surfaced past MinSimilarity (tag match justifies it).
	floored, err := svc.RecallMemories(ctx, "how does ranking work?", RecallOptions{
		Limit:                  5,
		MinSimilarity:          0.5,
		IncludeSubjectConcepts: true,
	})
	if err != nil {
		t.Fatalf("floored recall: %v", err)
	}
	foundConcept := false
	for _, r := range floored {
		if r.ID == "mem-concept-ridge" {
			foundConcept = true
		}
	}
	if !foundConcept {
		t.Fatalf("expected the ridge concept surfaced despite MinSimilarity, got %+v", floored)
	}
}

func TestRecallMemoriesMinSimilarityFilters(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	seedMemory(t, store, "mem-1", "aligned", []float64{1, 0})
	seedMemory(t, store, "mem-2", "orthogonal", []float64{0, 1})

	enricher := &stubEnricher{embed: []float64{1, 0}}
	svc := NewService(store, enricher, nil)

	results, err := svc.RecallMemories(context.Background(), "q", RecallOptions{MinSimilarity: 0.5})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(results) != 1 || results[0].ID != "mem-1" {
		t.Fatalf("expected only the aligned memory above floor, got %+v", results)
	}
}

func TestRecallMemoriesLimitClamped(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	for _, id := range []string{"mem-1", "mem-2", "mem-3"} {
		seedMemory(t, store, id, id, []float64{1, 0})
	}
	enricher := &stubEnricher{embed: []float64{1, 0}}
	svc := NewService(store, enricher, nil)

	results, err := svc.RecallMemories(context.Background(), "q", RecallOptions{Limit: 2})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected limit of 2 honored, got %d", len(results))
	}
}

func TestRecallMemoriesRequiresQuery(t *testing.T) {
	svc := NewService(issues.NewInMemoryStore(nil), &stubEnricher{embed: []float64{1, 0}}, nil)
	if _, err := svc.RecallMemories(context.Background(), "   ", RecallOptions{}); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestRecallMemoriesRequiresEnricher(t *testing.T) {
	svc := NewService(issues.NewInMemoryStore(nil), nil, nil)
	if _, err := svc.RecallMemories(context.Background(), "q", RecallOptions{}); err == nil {
		t.Fatal("expected error when no enricher is configured")
	}
}

func TestRecallMemoriesSurfacesEmbedError(t *testing.T) {
	enricher := &stubEnricher{embedErr: errors.New("embedder down")}
	svc := NewService(issues.NewInMemoryStore(nil), enricher, nil)
	if _, err := svc.RecallMemories(context.Background(), "q", RecallOptions{}); err == nil {
		t.Fatal("expected embed error to surface")
	}
}
