package issues

import (
	"testing"

	"sortit/internal/domain"
)

func TestComputeIssueLifecycleMetricsNilForInsufficientSnapshots(t *testing.T) {
	if metrics := ComputeIssueLifecycleMetrics(nil); metrics != nil {
		t.Fatalf("ComputeIssueLifecycleMetrics(nil) = %#v, want nil", metrics)
	}
	if metrics := ComputeIssueLifecycleMetrics([]IssueSnapshot{{Sequence: 1}}); metrics != nil {
		t.Fatalf("ComputeIssueLifecycleMetrics(single) = %#v, want nil", metrics)
	}
}

func TestComputeIssueLifecycleMetricsPrefersConvergedIssue(t *testing.T) {
	converged := ComputeIssueLifecycleMetrics([]IssueSnapshot{
		{Sequence: 1, Embedding: []float64{1, 0, 0}, TagScores: []domain.TagRelevance{{Tag: "search", Relevance: 0.9}}},
		{Sequence: 2, Embedding: []float64{0.95, 0.05, 0}, TagScores: []domain.TagRelevance{{Tag: "search", Relevance: 0.85}}},
		{Sequence: 3, Embedding: []float64{0.94, 0.06, 0}, TagScores: []domain.TagRelevance{{Tag: "search", Relevance: 0.86}}},
	})
	churny := ComputeIssueLifecycleMetrics([]IssueSnapshot{
		{Sequence: 1, Embedding: []float64{1, 0, 0}, TagScores: []domain.TagRelevance{{Tag: "search", Relevance: 0.9}}},
		{Sequence: 2, Embedding: []float64{0, 1, 0}, TagScores: []domain.TagRelevance{{Tag: "export", Relevance: 0.9}}},
		{Sequence: 3, Embedding: []float64{0.5, 0.5, 0}, TagScores: []domain.TagRelevance{{Tag: "search", Relevance: 0.5}, {Tag: "export", Relevance: 0.5}}},
	})

	if converged == nil || churny == nil {
		t.Fatalf("expected non-nil metrics, got converged=%#v churny=%#v", converged, churny)
	}
	if *converged.Churn >= *churny.Churn {
		t.Fatalf("expected converged churn %v < churny churn %v", *converged.Churn, *churny.Churn)
	}
}

func TestComputeIssueLifecycleMetricsWeightsRecentTransitionsMoreHeavily(t *testing.T) {
	stableThenChurny := ComputeIssueLifecycleMetrics([]IssueSnapshot{
		{Sequence: 1, Embedding: []float64{1, 0, 0}},
		{Sequence: 2, Embedding: []float64{0.99, 0.01, 0}},
		{Sequence: 3, Embedding: []float64{0.98, 0.02, 0}},
		{Sequence: 4, Embedding: []float64{0, 1, 0}},
	})
	churnyThenStable := ComputeIssueLifecycleMetrics([]IssueSnapshot{
		{Sequence: 1, Embedding: []float64{1, 0, 0}},
		{Sequence: 2, Embedding: []float64{0, 1, 0}},
		{Sequence: 3, Embedding: []float64{0.01, 0.99, 0}},
		{Sequence: 4, Embedding: []float64{0.02, 0.98, 0}},
	})

	if stableThenChurny == nil || churnyThenStable == nil {
		t.Fatalf("expected metrics for both histories")
	}
	if *stableThenChurny.Churn <= *churnyThenStable.Churn {
		t.Fatalf("expected recent churn to dominate: %v <= %v", *stableThenChurny.Churn, *churnyThenStable.Churn)
	}
}
