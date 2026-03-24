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
	if !names["cleanup"] {
		t.Fatal("expected cleanup anchor to remain available in the shortlist")
	}
	if !hints["database"] {
		t.Fatal("expected nearest shortlisted tag to be marked as a high-affinity hint")
	}
	if hints["cleanup"] {
		t.Fatal("expected stale-embedding candidate cleanup not to be marked as a high-affinity hint")
	}
}

func TestIssueTagScoresFromAnalysisAppliesServerSideFloor(t *testing.T) {
	scores := IssueTagScoresFromAnalysis([]ai.TagScore{
		{Tag: "below-floor", Relevance: 0.079},
		{Tag: "at-floor", Relevance: 0.08, Suggested: true, Description: " kept suggested description "},
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
	if !scores[0].Suggested {
		t.Fatal("expected suggested metadata to be preserved")
	}
	if scores[0].Description != "kept suggested description" {
		t.Fatalf("expected trimmed description to be preserved, got %q", scores[0].Description)
	}
	if scores[1].Suggested {
		t.Fatal("expected taxonomy tag to remain non-suggested")
	}
	if scores[1].Description != "" {
		t.Fatalf("expected empty description for non-suggested tag, got %q", scores[1].Description)
	}
}

func TestAnalyzeCreateInputUsesShortlistAndKeepsExplicitTagsAsAnchors(t *testing.T) {
	store := &catalogTestStore{
		tags: []issues.Tag{
			{Name: "backend", Embedding: []float64{0.3, 0.3}},
			{Name: "cleanup", Embedding: []float64{0.2, 0.2}},
			{Name: "database", Embedding: []float64{1, 0}},
			{Name: "billing", Embedding: []float64{0, 1}},
		},
	}
	tagger := &enricherTestTagger{}
	embedder := &enricherTestEmbedder{
		vectors: map[string][]float32{
			"database migration": {1, 0},
		},
	}
	analyzer := ai.NewAnalyzer(tagger, embedder)
	catalog := NewCatalogService(store, analyzer, slog.Default())
	enricher := NewIssueEnricher(analyzer, catalog, slog.Default())

	input, err := enricher.AnalyzeCreateInput(context.Background(), issues.CreateInput{
		Raw:  "database migration",
		Tags: []string{"cleanup"},
	})
	if err != nil {
		t.Fatalf("AnalyzeCreateInput: %v", err)
	}

	if len(embedder.calls) != 2 {
		t.Fatalf("expected 2 embed calls, got %d", len(embedder.calls))
	}

	names := make(map[string]bool)
	for _, tag := range tagger.capturedTags {
		names[tag.Name] = true
	}
	for _, want := range []string{"cleanup", "database", "backend"} {
		if !names[want] {
			t.Fatalf("expected create taxonomy to contain %q, got %#v", want, tagger.capturedTags)
		}
	}
	if names["billing"] {
		t.Fatalf("expected distant tag billing to be excluded, got %#v", tagger.capturedTags)
	}

	if len(input.Tags) != 1 || input.Tags[0] != "cleanup" {
		t.Fatalf("expected explicit tags to be preserved, got %#v", input.Tags)
	}
}
