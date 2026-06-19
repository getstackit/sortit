package enrichment

import (
	"context"
	"log/slog"
	"testing"

	"sortit/internal/ai"
	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

type capturingTagger struct {
	capturedPrior []ai.PriorDecision
}

func (t *capturingTagger) Score(_ context.Context, _ string, _ []ai.Tag, _ []ai.FewShotExample, prior []ai.PriorDecision) (ai.ScoreResult, error) {
	t.capturedPrior = append([]ai.PriorDecision(nil), prior...)
	return ai.ScoreResult{Tags: []ai.TagScore{{Tag: "search", Relevance: 0.9}}}, nil
}

func (t *capturingTagger) Provider() string { return testProviderName }
func (t *capturingTagger) Model() string    { return testProviderName }

func newMemoryContextEnricher(t *testing.T, tagger ai.Tagger) *IssueEnricher {
	t.Helper()
	catalogStore := &catalogTestStore{tags: []issues.Tag{
		{Name: "search", Embedding: []float64{1, 0}},
		{Name: "backend", Embedding: []float64{0, 1}},
	}}
	embedder := &enricherTestEmbedder{vectors: map[string][]float32{
		"ridge question": {1, 0},
	}}
	analyzer := ai.NewAnalyzer(tagger, embedder)
	catalog := tags.NewCatalogService(catalogStore, analyzer, slog.Default())
	enricher := NewIssueEnricher(analyzer, catalog, slog.Default())
	enricher.SetExemplarPool(nil)

	memStore := issues.NewInMemoryStore(nil)
	if err := memStore.UpsertMemory(context.Background(), domain.Memory{
		ID:         "m1",
		Title:      "Ridge is the default",
		Body:       "Issue search defaults to the ridge similarity model.",
		Status:     domain.MemoryStatusActive,
		AnchorTags: []string{"search"},
		Embedding:  []float64{1, 0},
	}); err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	enricher.UseMemoryContext(memStore)
	return enricher
}

func TestAnalyzeTextInjectsPriorDecisions(t *testing.T) {
	tagger := &capturingTagger{}
	enricher := newMemoryContextEnricher(t, tagger)

	if _, err := enricher.AnalyzeText(context.Background(), "ridge question", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
	}); err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	if len(tagger.capturedPrior) != 1 {
		t.Fatalf("expected 1 prior decision injected, got %d", len(tagger.capturedPrior))
	}
	if tagger.capturedPrior[0].Title != "Ridge is the default" {
		t.Fatalf("unexpected prior decision: %+v", tagger.capturedPrior[0])
	}
}

func TestAnalyzeTextSkipsPriorDecisions(t *testing.T) {
	tagger := &capturingTagger{}
	enricher := newMemoryContextEnricher(t, tagger)

	if _, err := enricher.AnalyzeText(context.Background(), "ridge question", AnalyzeTextOptions{
		CandidateMode:      tags.CandidateModeRetrievalShortlist,
		SkipPriorDecisions: true,
	}); err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	if len(tagger.capturedPrior) != 0 {
		t.Fatalf("expected no prior decisions when skipped, got %d", len(tagger.capturedPrior))
	}
}
