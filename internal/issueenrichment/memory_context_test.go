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
	capturedFrame ai.ConceptFrame
}

func (t *capturingTagger) Score(_ context.Context, _ string, _ []ai.Tag, _ []ai.FewShotExample, prior []ai.PriorDecision, frame ai.ConceptFrame) (ai.ScoreResult, error) {
	t.capturedPrior = append([]ai.PriorDecision(nil), prior...)
	t.capturedFrame = frame
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

// newConceptFrameEnricher seeds an active overview and concept (neither with an
// embedding, so they never leak into the prior-decisions similarity search) so
// the frame assembly has something to surface.
func newConceptFrameEnricher(t *testing.T, tagger ai.Tagger) *IssueEnricher {
	t.Helper()
	enricher := newMemoryContextEnricher(t, tagger)
	memStore := issues.NewInMemoryStore(nil)
	if err := memStore.UpsertMemory(context.Background(), domain.Memory{
		ID:     "ov",
		Title:  "Sortit",
		Body:   "Sortit is an issue tracker built around a factor model over tag relevance.",
		Kind:   domain.MemoryKindOverview,
		Status: domain.MemoryStatusActive,
	}); err != nil {
		t.Fatalf("seed overview: %v", err)
	}
	if err := memStore.UpsertMemory(context.Background(), domain.Memory{
		ID:         "c1",
		Title:      "Ridge regression",
		Body:       "Our search ranking uses diagonal-penalty ridge regression.",
		Kind:       domain.MemoryKindConcept,
		SubjectTag: "ridge regression",
		Status:     domain.MemoryStatusActive,
	}); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	enricher.UseMemoryContext(memStore)
	return enricher
}

func TestAnalyzeTextInjectsConceptFrame(t *testing.T) {
	tagger := &capturingTagger{}
	enricher := newConceptFrameEnricher(t, tagger)

	if _, err := enricher.AnalyzeText(context.Background(), "ridge question", AnalyzeTextOptions{
		CandidateMode: tags.CandidateModeRetrievalShortlist,
	}); err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	frame := tagger.capturedFrame
	if frame.IsEmpty() {
		t.Fatal("expected a non-empty concept frame to reach the tagger")
	}
	if frame.Overview != "Sortit is an issue tracker built around a factor model over tag relevance." {
		t.Fatalf("unexpected overview in frame: %q", frame.Overview)
	}
	if len(frame.Concepts) != 1 || frame.Concepts[0].SubjectTag != "ridge regression" {
		t.Fatalf("expected the seeded concept in the frame, got %+v", frame.Concepts)
	}
	if frame.Concepts[0].Profile != "Our search ranking uses diagonal-penalty ridge regression." {
		t.Fatalf("unexpected concept profile: %q", frame.Concepts[0].Profile)
	}
}

func TestAnalyzeTextSkipsConceptFrame(t *testing.T) {
	tagger := &capturingTagger{}
	enricher := newConceptFrameEnricher(t, tagger)

	if _, err := enricher.AnalyzeText(context.Background(), "ridge question", AnalyzeTextOptions{
		CandidateMode:      tags.CandidateModeRetrievalShortlist,
		SkipPriorDecisions: true,
	}); err != nil {
		t.Fatalf("AnalyzeText: %v", err)
	}

	if !tagger.capturedFrame.IsEmpty() {
		t.Fatalf("expected an empty frame when prior decisions are skipped, got %+v", tagger.capturedFrame)
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
