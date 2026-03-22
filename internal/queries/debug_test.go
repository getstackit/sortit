package queries

import (
	"context"
	"log/slog"
	"math"
	"testing"
	"time"

	"splat/internal/ai"
	"splat/internal/issues"
	"splat/internal/services"
)

type debugTagStore struct {
	tags []issues.Tag
}

func (s *debugTagStore) ListTags(context.Context) ([]issues.Tag, error) {
	out := make([]issues.Tag, len(s.tags))
	copy(out, s.tags)
	return out, nil
}

func (s *debugTagStore) UpsertTags(_ context.Context, tags []issues.Tag) error {
	out := make([]issues.Tag, len(tags))
	copy(out, tags)
	s.tags = out
	return nil
}

func (s *debugTagStore) UpdateTagSpecificity(context.Context, string, *float64, *float64, *float64, *time.Time) error {
	return nil
}

func TestDebugFactorWeightsReportsFallbackState(t *testing.T) {
	ctx := context.Background()
	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "issue-a", Raw: "a", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-b", Raw: "b", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-c", Raw: "c", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-d", Raw: "d", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "issue-e", Raw: "e", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}},
		{ID: "issue-f", Raw: "f", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}},
	})
	tagStore := &debugTagStore{}
	catalog := services.NewCatalogService(tagStore, nil, nil)
	if err := catalog.EnsureStoredTags(ctx, []issues.Tag{
		{Name: "alpha", Embedding: unitVec64([]float64{1, 0, 0, 0})},
	}); err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	result, err := (DebugFactorWeightsHandler{Store: store, Catalog: catalog}).Handle(ctx)
	if err != nil {
		t.Fatalf("handle debug factor weights: %v", err)
	}
	if result.Decomposed {
		t.Fatal("expected factor decomposition to report fallback state")
	}
	if result.DecomposedCount != 0 {
		t.Fatalf("expected 0 decomposed issues, got %d", result.DecomposedCount)
	}
}

func TestDebugIssueR2ReportsRawNorms(t *testing.T) {
	ctx := context.Background()
	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "pure-a", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{ID: "mixed-a", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 1, 0})},
		{ID: "mixed-b", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 1, 0})},
		{ID: "mixed-c", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 1, 0})},
		{ID: "mixed-d", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}, Embedding: unitVec64([]float64{1, 0, 1, 0})},
	})
	tagStore := &debugTagStore{}
	catalog := services.NewCatalogService(tagStore, nil, nil)
	if err := catalog.EnsureStoredTags(ctx, []issues.Tag{
		{Name: "alpha", Embedding: unitVec64([]float64{1, 0, 0, 0})},
	}); err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	result, err := (DebugIssueR2Handler{Store: store, Catalog: catalog}).Handle(ctx, "mixed-a")
	if err != nil {
		t.Fatalf("handle debug issue r2: %v", err)
	}
	if result.Skipped {
		t.Fatalf("expected decomposition-backed diagnosis, got skipped=%v reason=%q", result.Skipped, result.SkipReason)
	}

	want := math.Round(math.Sqrt(0.5)*1000) / 1000
	if result.ExplainedNorm != want {
		t.Fatalf("expected explained norm %v, got %v", want, result.ExplainedNorm)
	}
	if result.ResidualNorm != want {
		t.Fatalf("expected residual norm %v, got %v", want, result.ResidualNorm)
	}
}

type debugEvalTagger struct {
	scoresByText map[string][]ai.TagScore
}

func (t *debugEvalTagger) Score(_ context.Context, text string, _ []ai.Tag) ([]ai.TagScore, error) {
	return append([]ai.TagScore(nil), t.scoresByText[text]...), nil
}

func (t *debugEvalTagger) Provider() string {
	return "fake"
}

func (t *debugEvalTagger) Model() string {
	return "eval-tagger"
}

type debugEvalEmbedder struct{}

func (e *debugEvalEmbedder) EmbedText(context.Context, string) (ai.EmbeddingResult, error) {
	return ai.EmbeddingResult{}, nil
}

func (e *debugEvalEmbedder) Provider() string {
	return "fake"
}

func (e *debugEvalEmbedder) Model() string {
	return "eval-embedder"
}

func TestDebugEvalTagsReportsAggregateMetrics(t *testing.T) {
	ctx := context.Background()
	analyzer := ai.NewAnalyzer(&debugEvalTagger{
		scoresByText: map[string][]ai.TagScore{
			"issue a": {
				{Tag: "export", Relevance: 0.91},
				{Tag: "bug", Relevance: 0.44},
				{Tag: "noise", Relevance: 0.07},
			},
			"issue b": {
				{Tag: "search", Relevance: 0.95},
			},
		},
	}, &debugEvalEmbedder{})
	catalog := services.NewCatalogService(&debugTagStore{}, analyzer, nil)
	handler := DebugEvalTagsHandler{
		Catalog:  catalog,
		Enricher: services.NewIssueEnricher(analyzer, catalog, slog.Default()),
		Fixture: []DebugTagEvalCase{
			{ID: "a", Text: "issue a", ExpectedTags: []string{"export", "safari"}},
			{ID: "b", Text: "issue b", ExpectedTags: []string{"search"}},
		},
	}

	result, err := handler.Handle(ctx)
	if err != nil {
		t.Fatalf("handle debug eval tags: %v", err)
	}
	if result.CaseCount != 2 {
		t.Fatalf("expected 2 cases, got %d", result.CaseCount)
	}
	if result.Precision != 0.667 {
		t.Fatalf("expected aggregate precision 0.667, got %v", result.Precision)
	}
	if result.Recall != 0.667 {
		t.Fatalf("expected aggregate recall 0.667, got %v", result.Recall)
	}
	if result.ExactMatchCount != 1 {
		t.Fatalf("expected 1 exact match, got %d", result.ExactMatchCount)
	}
	if got := result.Cases[0].MissingTags; len(got) != 1 || got[0] != "safari" {
		t.Fatalf("expected missing safari, got %+v", got)
	}
	if got := result.Cases[0].UnexpectedTags; len(got) != 1 || got[0] != "bug" {
		t.Fatalf("expected unexpected bug, got %+v", got)
	}
	if got := result.Cases[0].ActualTags; len(got) != 2 || got[0] != "export" || got[1] != "bug" {
		t.Fatalf("unexpected actual tags %+v", got)
	}
}

func unitVec64(v []float64) []float64 {
	out := make([]float64, len(v))
	copy(out, v)
	var mag float64
	for _, val := range out {
		mag += val * val
	}
	if mag > 0 {
		mag = math.Sqrt(mag)
		for i := range out {
			out[i] /= mag
		}
	}
	return out
}
