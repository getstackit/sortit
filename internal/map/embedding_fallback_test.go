package issuemap

import (
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

func persistedEmbedding(axis int) []float64 {
	v := make([]float64, embeddingDimensions)
	v[axis] = 1
	return v
}

// Counter assertions use before/after deltas: the tracker is package-global
// and other tests in this package legitimately trip the fallback.
func TestEmbeddingFallbackCountsMissingIssueEmbedding(t *testing.T) {
	before := EmbeddingFallbackCounts()

	storeIssues := []issues.Issue{
		{
			ID:        "has-embedding",
			Raw:       "issue with a persisted embedding",
			Embedding: persistedEmbedding(0),
			TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.9}},
		},
		{
			ID:        "missing-embedding",
			Raw:       "issue without a persisted embedding",
			TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.8}},
		},
	}
	storeTags := []issues.Tag{{Name: "bug", Embedding: persistedEmbedding(1)}}

	runtimeMapInputs(storeIssues, storeTags)

	after := EmbeddingFallbackCounts()
	if got := after[EmbeddingFallbackKindIssue] - before[EmbeddingFallbackKindIssue]; got != 1 {
		t.Fatalf("expected exactly 1 issue fallback, got %d", got)
	}
	if got := after[EmbeddingFallbackKindTag] - before[EmbeddingFallbackKindTag]; got != 0 {
		t.Fatalf("expected no tag fallbacks, got %d", got)
	}
}

func TestEmbeddingFallbackCountsMissingTagEmbedding(t *testing.T) {
	before := EmbeddingFallbackCounts()

	storeIssues := []issues.Issue{
		{
			ID:        "issue-1",
			Raw:       "issue with a persisted embedding",
			Embedding: persistedEmbedding(0),
			TagScores: []domain.TagRelevance{{Tag: "crash", Relevance: 0.9}},
		},
	}
	storeTags := []issues.Tag{{Name: "crash"}} // no stored embedding

	runtimeMapInputs(storeIssues, storeTags)

	after := EmbeddingFallbackCounts()
	if got := after[EmbeddingFallbackKindTag] - before[EmbeddingFallbackKindTag]; got != 1 {
		t.Fatalf("expected exactly 1 tag fallback, got %d", got)
	}
	if got := after[EmbeddingFallbackKindIssue] - before[EmbeddingFallbackKindIssue]; got != 0 {
		t.Fatalf("expected no issue fallbacks, got %d", got)
	}
}

func TestEmbeddingFallbackZeroWhenAllEmbeddingsPresent(t *testing.T) {
	before := EmbeddingFallbackCounts()

	storeIssues := []issues.Issue{
		{
			ID:        "issue-1",
			Raw:       "first fully-embedded issue",
			Embedding: persistedEmbedding(0),
			TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.9}},
		},
		{
			ID:        "issue-2",
			Raw:       "second fully-embedded issue",
			Embedding: persistedEmbedding(1),
			TagScores: []domain.TagRelevance{{Tag: "frontend", Relevance: 0.7}},
		},
	}
	storeTags := []issues.Tag{
		{Name: "bug", Embedding: persistedEmbedding(2)},
		{Name: "frontend", Embedding: persistedEmbedding(3)},
	}

	runtimeMapInputs(storeIssues, storeTags)

	after := EmbeddingFallbackCounts()
	for _, kind := range []string{EmbeddingFallbackKindIssue, EmbeddingFallbackKindTag} {
		if got := after[kind] - before[kind]; got != 0 {
			t.Fatalf("expected no %s fallbacks with all embeddings persisted, got %d", kind, got)
		}
	}
}
