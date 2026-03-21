package queries

import (
	"context"
	"math"
	"testing"
	"time"

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
