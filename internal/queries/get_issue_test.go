package queries

import (
	"context"
	"testing"

	"splat/internal/issues"
)

func TestGetIssueHandlerIncludesLifecycleMetricsFromSnapshots(t *testing.T) {
	store := issues.NewInMemoryStore(nil)
	issue := issues.BuildNewIssue("issue-123", issues.CreateInput{
		Raw:       "Export fails on iPad.",
		CreatedBy: "Casey",
	})
	if err := store.SaveIssue(context.Background(), issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	raw1 := "Export fails on iPad in Safari."
	seq1 := 1
	if err := store.UpdateIssueFields(context.Background(), issue.ID, issues.IssueFieldUpdate{
		Raw:  &raw1,
		Tags: []string{"export", "safari"},
		TagScores: []issues.TagRelevance{
			{Tag: "export", Relevance: 0.9},
			{Tag: "safari", Relevance: 0.7},
		},
		Embedding:                []float64{1, 0},
		EnrichmentTargetSequence: &seq1,
	}); err != nil {
		t.Fatalf("update issue fields seq1: %v", err)
	}

	raw2 := "Export fails on iPad in Safari after tapping share twice."
	seq2 := 2
	if err := store.UpdateIssueFields(context.Background(), issue.ID, issues.IssueFieldUpdate{
		Raw:  &raw2,
		Tags: []string{"export", "safari"},
		TagScores: []issues.TagRelevance{
			{Tag: "export", Relevance: 0.95},
			{Tag: "safari", Relevance: 0.8},
		},
		Embedding:                []float64{0.8, 0.2},
		EnrichmentTargetSequence: &seq2,
	}); err != nil {
		t.Fatalf("update issue fields seq2: %v", err)
	}

	handler := GetIssueHandler{Store: store}
	loaded, err := handler.Handle(context.Background(), GetIssue{ID: issue.ID})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if loaded.LifecycleMetrics == nil {
		t.Fatal("expected lifecycle metrics")
	}
	if loaded.LifecycleMetrics.TransitionCount != 1 {
		t.Fatalf("expected 1 transition, got %#v", loaded.LifecycleMetrics)
	}
	if loaded.LifecycleMetrics.Churn == nil || loaded.LifecycleMetrics.Stability == nil {
		t.Fatalf("expected non-nil churn/stability, got %#v", loaded.LifecycleMetrics)
	}
	if *loaded.LifecycleMetrics.Churn <= 0 {
		t.Fatalf("expected positive churn, got %#v", loaded.LifecycleMetrics)
	}
}
