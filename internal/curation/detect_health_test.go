package curation

import (
	"context"
	"testing"

	"sortit/internal/diagnostics"
	"sortit/internal/issues"
)

type fakeFactorWeights struct {
	result diagnostics.DebugFactorWeightsResult
	err    error
}

func (f fakeFactorWeights) Handle(_ context.Context) (diagnostics.DebugFactorWeightsResult, error) {
	return f.result, f.err
}

func TestDetectHealthIssuesMergesFailedAndLowR2(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "fail", Raw: "enrichment broke", Status: issues.StatusOpen, EnrichmentStatus: issues.EnrichmentStatusFailed, EnrichmentError: "boom"},
		{ID: "ok", Raw: "all good", Status: issues.StatusOpen, EnrichmentStatus: issues.EnrichmentStatusComplete},
	})

	fw := fakeFactorWeights{result: diagnostics.DebugFactorWeightsResult{
		LowR2Issues: []diagnostics.DebugLowR2Issue{
			{ID: "lowr2", Raw: "poorly explained by tags", R2: 0.10},
			{ID: "fail", Raw: "enrichment broke", R2: 0.05}, // already surfaced as enrichment_failed
		},
		ReviewQueue: diagnostics.DebugReviewQueue{
			LowAlignmentTags: []diagnostics.DebugReviewLowAlignment{
				{IssueID: "x", TagScore: issues.TagRelevance{Tag: "safari"}, Alignment: 0.2},
			},
		},
	}}

	det := NewDetector(store, store, fakeExplorer{}, fw, nil)
	report, err := det.DetectHealthIssues(ctx, HealthParams{IncludeFailed: true})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	byID := map[string]HealthIssue{}
	for _, h := range report.Issues {
		if _, dup := byID[h.IssueID]; dup {
			t.Fatalf("issue %q surfaced twice (should dedupe)", h.IssueID)
		}
		byID[h.IssueID] = h
	}
	if len(byID) != 2 {
		t.Fatalf("expected 2 distinct health issues, got %d: %+v", len(byID), report.Issues)
	}
	if byID["fail"].Reason != "enrichment_failed" || byID["fail"].EnrichmentError != "boom" {
		t.Fatalf("unexpected fail entry: %+v", byID["fail"])
	}
	if byID["lowr2"].Reason != "low_r2" || byID["lowr2"].R2 == nil || *byID["lowr2"].R2 != 0.10 {
		t.Fatalf("unexpected lowr2 entry: %+v", byID["lowr2"])
	}
	if len(report.TaxonomyGaps) != 1 || report.TaxonomyGaps[0].Tag != "safari" || report.TaxonomyGaps[0].IssueID != "x" {
		t.Fatalf("unexpected taxonomy gaps: %+v", report.TaxonomyGaps)
	}
}

func TestDetectHealthIssuesWithoutFactorWeights(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "fail", Raw: "broke", Status: issues.StatusOpen, EnrichmentStatus: issues.EnrichmentStatusFailed, EnrichmentError: "boom"},
	})

	det := NewDetector(store, store, fakeExplorer{}, nil, nil)
	report, err := det.DetectHealthIssues(ctx, HealthParams{IncludeFailed: true})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(report.Issues) != 1 || report.Issues[0].Reason != "enrichment_failed" {
		t.Fatalf("expected only enrichment_failed without factor weights, got %+v", report.Issues)
	}
	if len(report.TaxonomyGaps) != 0 {
		t.Fatalf("expected no taxonomy gaps without factor weights, got %+v", report.TaxonomyGaps)
	}
}
