package issuemath

import (
	"testing"

	"splat/internal/issues"
)

func TestMeanTagProfileWeightsSpecificTagsMoreHighly(t *testing.T) {
	specificity := map[string]*float64{
		"specific": floatPtr(0.9),
		"generic":  floatPtr(0.2),
	}
	items := []issues.PeopleAnalyticsIssue{
		{
			TagScores: []issues.TagRelevance{
				{Tag: "specific", Relevance: 0.8},
				{Tag: "generic", Relevance: 0.8},
			},
		},
		{
			TagScores: []issues.TagRelevance{
				{Tag: "specific", Relevance: 0.6},
				{Tag: "generic", Relevance: 0.6},
			},
		},
	}

	profile := MeanTagProfile(items, specificity)

	if len(profile) != 2 {
		t.Fatalf("expected 2 profile entries, got %d", len(profile))
	}
	if profile[0].Tag != "specific" {
		t.Fatalf("expected specific tag to rank first, got %#v", profile)
	}
	if profile[0].Relevance <= profile[1].Relevance {
		t.Fatalf("expected specific tag relevance %v > generic %v", profile[0].Relevance, profile[1].Relevance)
	}
}

func TestMeanEmbeddingNormalizesAveragedVector(t *testing.T) {
	items := []issues.PeopleAnalyticsIssue{
		{Embedding: []float64{1, 0}},
		{Embedding: []float64{0, 1}},
	}

	mean := MeanEmbedding(items)

	if len(mean) != 2 {
		t.Fatalf("expected embedding length 2, got %d", len(mean))
	}
	const want = 0.70710678
	if mean[0] < want-0.01 || mean[0] > want+0.01 || mean[1] < want-0.01 || mean[1] > want+0.01 {
		t.Fatalf("unexpected normalized mean embedding: %#v", mean)
	}
}

func TestTagProfileSimilarity(t *testing.T) {
	a := []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}
	b := []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}
	c := []issues.TagRelevance{{Tag: "beta", Relevance: 1}}

	if got := TagProfileSimilarity(a, b); got < 0.99 {
		t.Fatalf("expected matching profiles similarity near 1, got %v", got)
	}
	if got := TagProfileSimilarity(a, c); got != 0 {
		t.Fatalf("expected disjoint profiles similarity 0, got %v", got)
	}
}

func floatPtr(v float64) *float64 {
	return &v
}
