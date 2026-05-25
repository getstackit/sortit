package tagcooccurrence

import (
	"testing"

	"sortit/internal/issues"
)

func TestComputeEmptyCorpus(t *testing.T) {
	got := Compute(nil)
	if got.IssueCount != 0 || got.TagCount != 0 || len(got.Tags) != 0 {
		t.Fatalf("expected empty stats, got %+v", got)
	}
}

func TestComputeIgnoresBelowFloor(t *testing.T) {
	items := []issues.Issue{
		{TagScores: []issues.TagRelevance{
			{Tag: "bug", Relevance: 0.9},
			{Tag: "noise", Relevance: 0.05}, // below floor — should be ignored
		}},
	}
	got := Compute(items)
	if got.TagCount != 1 {
		t.Fatalf("expected 1 tag above floor, got %d: %+v", got.TagCount, got)
	}
	if got.Tags[0].Tag != "bug" {
		t.Fatalf("expected bug, got %q", got.Tags[0].Tag)
	}
}

func TestComputeAntiCorrelation(t *testing.T) {
	// bug and feature never co-occur; given bug, conditional P(feature|bug) = 0
	// while base P(feature) = 1/4. Implicit-negative = 0.25.
	items := []issues.Issue{
		{TagScores: []issues.TagRelevance{{Tag: "bug", Relevance: 0.9}}},
		{TagScores: []issues.TagRelevance{{Tag: "bug", Relevance: 0.9}}},
		{TagScores: []issues.TagRelevance{{Tag: "bug", Relevance: 0.9}}},
		{TagScores: []issues.TagRelevance{{Tag: "feature", Relevance: 0.9}}},
	}
	got := Compute(items)
	bug, ok := got.LookupTag("bug")
	if !ok {
		t.Fatalf("expected bug stats")
	}
	if bug.Count != 3 {
		t.Fatalf("expected bug count 3, got %d", bug.Count)
	}
	if len(bug.Pairs) != 1 || bug.Pairs[0].Tag != "feature" {
		t.Fatalf("expected single pair against feature, got %+v", bug.Pairs)
	}
	if bug.Pairs[0].ImplicitNegative != 0.25 {
		t.Fatalf("expected implicit-negative 0.25, got %v", bug.Pairs[0].ImplicitNegative)
	}
	if bug.Pairs[0].ConditionalProb != 0 {
		t.Fatalf("expected conditional 0, got %v", bug.Pairs[0].ConditionalProb)
	}
}

func TestComputeFullCorrelation(t *testing.T) {
	// When two tags always co-occur, conditional == base, implicit-negative = 0.
	items := []issues.Issue{
		{TagScores: []issues.TagRelevance{
			{Tag: "bug", Relevance: 0.9},
			{Tag: "crash", Relevance: 0.9},
		}},
		{TagScores: []issues.TagRelevance{
			{Tag: "bug", Relevance: 0.9},
			{Tag: "crash", Relevance: 0.9},
		}},
	}
	got := Compute(items)
	bug, _ := got.LookupTag("bug")
	if len(bug.Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(bug.Pairs))
	}
	if bug.Pairs[0].ImplicitNegative != 0 {
		t.Fatalf("expected implicit-negative 0 for fully-correlated pair, got %v", bug.Pairs[0].ImplicitNegative)
	}
	if bug.Pairs[0].ConditionalProb != 1 {
		t.Fatalf("expected conditional 1, got %v", bug.Pairs[0].ConditionalProb)
	}
}

func TestComputeNormalizesTagNames(t *testing.T) {
	items := []issues.Issue{
		{TagScores: []issues.TagRelevance{{Tag: " Bug ", Relevance: 0.9}}},
		{TagScores: []issues.TagRelevance{{Tag: "bug", Relevance: 0.9}}},
	}
	got := Compute(items)
	if got.TagCount != 1 {
		t.Fatalf("expected normalized to 1 tag, got %d: %+v", got.TagCount, got)
	}
	if got.Tags[0].Count != 2 {
		t.Fatalf("expected normalized count 2, got %d", got.Tags[0].Count)
	}
}
