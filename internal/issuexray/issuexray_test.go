package issuexray

import (
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/tagcooccurrence"
)

func TestComputeSpecificityWeightedMean(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	issue := issues.Issue{
		TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.8},
			{Tag: "billing", Relevance: 0.2},
		},
	}
	tagSpec := map[string]float64{"auth": 0.9, "billing": 0.3}

	out := Compute(issue, tagSpec, nil, now)

	if out.Specificity == nil {
		t.Fatal("expected non-nil specificity")
	}
	// 0.8*0.9 + 0.2*0.3 = 0.78; / total weight 1.0
	want := 0.78
	if *out.Specificity != want {
		t.Fatalf("specificity = %v, want %v", *out.Specificity, want)
	}
}

func TestComputeCoherenceRequiresTwoStrongTags(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	stats := tagcooccurrence.Compute([]issues.Issue{
		{TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.9},
			{Tag: "tokens", Relevance: 0.7},
		}},
		{TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.8},
			{Tag: "tokens", Relevance: 0.6},
		}},
	})

	issue := issues.Issue{
		TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}},
	}
	out := Compute(issue, nil, &stats, now)
	if out.Coherence != nil {
		t.Fatalf("expected nil coherence with one strong tag; got %+v", out.Coherence)
	}
}

func TestComputeCoherenceHighForFrequentlyCooccurringTags(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	corpus := []issues.Issue{
		{TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.9},
			{Tag: "tokens", Relevance: 0.7},
		}},
		{TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.8},
			{Tag: "tokens", Relevance: 0.6},
		}},
		{TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.6},
			{Tag: "tokens", Relevance: 0.6},
		}},
	}
	stats := tagcooccurrence.Compute(corpus)
	issue := issues.Issue{
		TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.9},
			{Tag: "tokens", Relevance: 0.7},
		},
	}
	out := Compute(issue, nil, &stats, now)
	if out.Coherence == nil {
		t.Fatal("expected non-nil coherence")
	}
	// Both tags always co-occur → conditional prob ≈ 1
	if *out.Coherence < 0.9 {
		t.Fatalf("expected coherence ≈ 1 for always-cooccurring tags; got %v", *out.Coherence)
	}
}

func TestComputeDefinitionFavorsDominantTag(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)

	dominant := issues.Issue{
		TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.9},
			{Tag: "other", Relevance: 0.1},
		},
	}
	spread := issues.Issue{
		TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.5},
			{Tag: "other", Relevance: 0.5},
		},
	}

	a := Compute(dominant, nil, nil, now)
	b := Compute(spread, nil, nil, now)
	if a.Definition == nil || b.Definition == nil {
		t.Fatal("expected definition values for both fixtures")
	}
	if *a.Definition <= *b.Definition {
		t.Fatalf("expected dominant issue to have higher definition; got %v vs %v", *a.Definition, *b.Definition)
	}
}

func TestComputeConnectednessSaturates(t *testing.T) {
	now := time.Now()
	issue := issues.Issue{
		Links:      make([]issues.IssueLink, 5),
		Operations: make([]issues.IssueOperation, 5),
	}
	out := Compute(issue, nil, nil, now)
	if out.Connectedness.Score != 1.0 {
		t.Fatalf("expected saturated score 1.0; got %v", out.Connectedness.Score)
	}
}

func TestComputeLifecycleEarly(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	issue := issues.Issue{
		CreatedAt: now.Add(-3 * 24 * time.Hour),
		LifecycleMetrics: &issues.IssueLifecycleMetrics{
			RefinementCount: 0,
		},
	}
	out := Compute(issue, nil, nil, now)
	if out.Lifecycle.Label != "early" {
		t.Fatalf("expected early; got %q", out.Lifecycle.Label)
	}
}

func TestComputeLifecycleLate(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	issue := issues.Issue{
		CreatedAt: now.Add(-60 * 24 * time.Hour),
		LifecycleMetrics: &issues.IssueLifecycleMetrics{
			RefinementCount: 6,
		},
	}
	out := Compute(issue, nil, nil, now)
	if out.Lifecycle.Label != "late" {
		t.Fatalf("expected late; got %q", out.Lifecycle.Label)
	}
}
