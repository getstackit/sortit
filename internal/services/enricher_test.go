package services

import (
	"context"
	"log/slog"
	"testing"

	"splat/internal/ai"
	"splat/internal/issues"
)

const testProviderName = "test"

type enricherTestTagger struct {
	capturedTags []ai.Tag
}

func (t *enricherTestTagger) Score(_ context.Context, _ string, tags []ai.Tag) ([]ai.TagScore, error) {
	t.capturedTags = append([]ai.Tag(nil), tags...)
	return []ai.TagScore{{Tag: "database", Relevance: 0.9}}, nil
}

func (t *enricherTestTagger) Provider() string {
	return testProviderName
}

func (t *enricherTestTagger) Model() string {
	return testProviderName
}

type enricherTestEmbedder struct {
	calls   []string
	vectors map[string][]float32
}

func (e *enricherTestEmbedder) EmbedText(_ context.Context, text string) (ai.EmbeddingResult, error) {
	e.calls = append(e.calls, text)
	vector := append([]float32(nil), e.vectors[text]...)
	return ai.EmbeddingResult{Vector: vector}, nil
}

func (e *enricherTestEmbedder) Provider() string {
	return testProviderName
}

func (e *enricherTestEmbedder) Model() string {
	return testProviderName
}

func TestAnalyzePersistedIssueUsesFreshEmbeddingForShortlist(t *testing.T) {
	store := &catalogTestStore{
		tags: []issues.Tag{
			{Name: "database", Embedding: []float64{1, 0}},
			{Name: "cleanup", Embedding: []float64{0, 1}},
		},
	}
	tagger := &enricherTestTagger{}
	embedder := &enricherTestEmbedder{
		vectors: map[string][]float32{
			"database issue": {1, 0},
		},
	}
	analyzer := ai.NewAnalyzer(tagger, embedder)
	catalog := NewCatalogService(store, analyzer, slog.Default())
	enricher := NewIssueEnricher(analyzer, catalog, slog.Default())

	_, err := enricher.AnalyzePersistedIssue(context.Background(), issues.Issue{
		ID:        "issue-1",
		Raw:       "database issue",
		Embedding: []float64{0, 1}, // stale embedding points at cleanup
	}, 1)
	if err != nil {
		t.Fatalf("AnalyzePersistedIssue: %v", err)
	}

	if len(embedder.calls) != 2 {
		t.Fatalf("expected 2 embed calls, got %d", len(embedder.calls))
	}
	if embedder.calls[0] != "database issue" {
		t.Fatalf("expected first embed call to use canonical raw, got %q", embedder.calls[0])
	}
	if embedder.calls[1] != "database issue" {
		t.Fatalf("expected analyzer embed call to use canonical raw, got %q", embedder.calls[1])
	}

	names := make(map[string]bool)
	hints := make(map[string]bool)
	for _, tag := range tagger.capturedTags {
		names[tag.Name] = true
		hints[tag.Name] = tag.Hint
	}
	if !names["database"] {
		t.Fatal("expected database to be shortlisted from fresh canonical embedding")
	}
	if names["cleanup"] {
		t.Fatal("expected cleanup not to be shortlisted from stale persisted embedding")
	}
	if !hints["database"] {
		t.Fatal("expected nearest shortlisted tag to be marked as a high-affinity hint")
	}
}

func TestIssueTagScoresFromAnalysisAppliesServerSideFloor(t *testing.T) {
	scores := IssueTagScoresFromAnalysis([]ai.TagScore{
		{Tag: "below-floor", Relevance: 0.079},
		{Tag: "at-floor", Relevance: 0.08},
		{Tag: "above-floor", Relevance: 0.81},
	})

	if len(scores) != 2 {
		t.Fatalf("expected 2 persisted scores, got %d", len(scores))
	}
	if scores[0].Tag != "at-floor" {
		t.Fatalf("expected at-floor to be preserved, got %q", scores[0].Tag)
	}
	if scores[1].Tag != "above-floor" {
		t.Fatalf("expected above-floor to be preserved, got %q", scores[1].Tag)
	}
}
