package issuethemes

import (
	"math"
	"testing"
)

func TestBuildDerivesDeterministicThemes(t *testing.T) {
	tagNames := []string{"auth", "login", "billing", "invoice"}
	loadings := []IssueLoading{
		{IssueID: "auth-1", Values: []float64{1.0, 0.8, 0, 0}},
		{IssueID: "auth-2", Values: []float64{0.9, 0.7, 0, 0}},
		{IssueID: "billing-1", Values: []float64{0, 0, 1.0, 0.8}},
		{IssueID: "billing-2", Values: []float64{0, 0, 0.9, 0.7}},
	}
	embeddings := map[string][]float64{
		"auth":    {1, 0, 0},
		"login":   {0.9, 0.1, 0},
		"billing": {0, 1, 0},
		"invoice": {0.1, 0.9, 0},
	}

	first := Build(loadings, tagNames, embeddings, Options{ThemeCount: 2, Iterations: 60, TopTags: 2})
	second := Build(loadings, tagNames, embeddings, Options{ThemeCount: 2, Iterations: 60, TopTags: 2})

	if len(first.Themes) != 2 || len(first.IssueThemes) != 4 {
		t.Fatalf("expected two themes and four issue rows, got %+v", first)
	}
	if !sameResult(first, second) {
		t.Fatalf("expected deterministic result\nfirst:  %+v\nsecond: %+v", first, second)
	}
	for _, theme := range first.Themes {
		if len(theme.Tags) != 2 {
			t.Fatalf("expected top two tags for theme %+v", theme)
		}
		if theme.Tags[0].Loading < theme.Tags[1].Loading {
			t.Fatalf("expected tags sorted by loading, got %+v", theme.Tags)
		}
		if got := norm(theme.Centroid); math.Abs(got-1) > 1e-9 {
			t.Fatalf("expected unit centroid, got norm %v for %+v", got, theme)
		}
	}

	firstTags := []string{first.Themes[0].Tags[0].Tag, first.Themes[1].Tags[0].Tag}
	if !contains(firstTags, "auth") || !contains(firstTags, "billing") {
		t.Fatalf("expected auth and billing themes, got %+v", first.Themes)
	}
}

func TestBuildUsesPositivePartsOnly(t *testing.T) {
	result := Build([]IssueLoading{
		{IssueID: "negated", Values: []float64{-0.8, 0.6}},
	}, []string{"backend", "signed-relevance"}, nil, Options{ThemeCount: 1, Iterations: 10})

	if len(result.Themes) != 1 {
		t.Fatalf("expected one positive theme, got %+v", result)
	}
	if result.Themes[0].Tags[0].Tag != "signed-relevance" {
		t.Fatalf("expected negative loading to be ignored, got %+v", result.Themes[0].Tags)
	}
}

func TestBuildHandlesEmptyAndDegenerateInputs(t *testing.T) {
	if got := Build(nil, []string{"auth"}, nil, Options{}); len(got.Themes) != 0 {
		t.Fatalf("expected empty result for no rows, got %+v", got)
	}
	if got := Build([]IssueLoading{{IssueID: "a", Values: []float64{-1}}}, []string{"auth"}, nil, Options{}); len(got.Themes) != 0 {
		t.Fatalf("expected empty result for no positive mass, got %+v", got)
	}
	if got := Build([]IssueLoading{{IssueID: "a", Values: []float64{1}}}, nil, nil, Options{}); len(got.Themes) != 0 {
		t.Fatalf("expected empty result for no tags, got %+v", got)
	}
}

func TestBuildCapsThemeCountToMatrixRankShape(t *testing.T) {
	result := Build([]IssueLoading{
		{IssueID: "one", Values: []float64{1, 0}},
	}, []string{"auth", "billing"}, nil, Options{ThemeCount: 8})

	if len(result.Themes) != 1 || len(result.IssueThemes[0].Weights) != 1 {
		t.Fatalf("expected theme count capped to one row, got %+v", result)
	}
}

func sameResult(a, b Result) bool {
	if len(a.Themes) != len(b.Themes) || len(a.IssueThemes) != len(b.IssueThemes) {
		return false
	}
	for i := range a.Themes {
		if math.Abs(a.Themes[i].Weight-b.Themes[i].Weight) > 1e-12 {
			return false
		}
		if len(a.Themes[i].Tags) != len(b.Themes[i].Tags) {
			return false
		}
		for j := range a.Themes[i].Tags {
			if a.Themes[i].Tags[j].Tag != b.Themes[i].Tags[j].Tag ||
				math.Abs(a.Themes[i].Tags[j].Loading-b.Themes[i].Tags[j].Loading) > 1e-12 {
				return false
			}
		}
	}
	for i := range a.IssueThemes {
		if a.IssueThemes[i].IssueID != b.IssueThemes[i].IssueID ||
			len(a.IssueThemes[i].Weights) != len(b.IssueThemes[i].Weights) {
			return false
		}
		for j := range a.IssueThemes[i].Weights {
			if math.Abs(a.IssueThemes[i].Weights[j]-b.IssueThemes[i].Weights[j]) > 1e-12 {
				return false
			}
		}
	}
	return true
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
