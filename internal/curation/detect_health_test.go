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

type fakeTagHealth struct {
	result diagnostics.DebugTagHealthResult
	err    error
}

func (f fakeTagHealth) Handle(_ context.Context) (diagnostics.DebugTagHealthResult, error) {
	return f.result, f.err
}

func TestDetectHealthIssuesDriftIsPrimaryAndDedupesLowR2(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "drift", Raw: "tagged alpha but really beta", Status: issues.StatusOpen, EnrichmentStatus: issues.EnrichmentStatusComplete},
		{ID: "novel", Raw: "concept no tag covers", Status: issues.StatusOpen, EnrichmentStatus: issues.EnrichmentStatusComplete},
	})

	th := fakeTagHealth{result: diagnostics.DebugTagHealthResult{
		Computed: true,
		HighDriftIssues: []diagnostics.DebugDriftIssue{{
			ID:           "drift",
			Raw:          "tagged alpha but really beta",
			DriftCosine:  0.30,
			SpuriousTags: []diagnostics.DebugDriftTag{{Tag: "alpha", Delta: -0.6}},
			MissingTags:  []diagnostics.DebugDriftTag{{Tag: "beta", Delta: 0.9}},
		}},
	}}
	// The rank-1 path also flags "drift" (low R²) — drift must win the dedup —
	// and uniquely flags "novel" (an uncovered concept drift doesn't catch).
	fw := fakeFactorWeights{result: diagnostics.DebugFactorWeightsResult{
		LowR2Issues: []diagnostics.DebugLowR2Issue{
			{ID: "drift", Raw: "tagged alpha but really beta", R2: 0.05},
			{ID: "novel", Raw: "concept no tag covers", R2: 0.08},
		},
	}}

	det := NewDetector(store, store, fakeExplorer{}, fw, th, nil)
	report, err := det.DetectHealthIssues(ctx, HealthParams{})
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

	drift := byID["drift"]
	if drift.Reason != "high_drift" {
		t.Fatalf("expected drift issue to win as high_drift, got %q", drift.Reason)
	}
	if drift.DriftCosine == nil || *drift.DriftCosine != 0.30 {
		t.Fatalf("expected drift cosine 0.30, got %+v", drift.DriftCosine)
	}
	if len(drift.SpuriousTags) != 1 || drift.SpuriousTags[0] != "alpha" {
		t.Fatalf("expected alpha spurious, got %+v", drift.SpuriousTags)
	}
	if len(drift.MissingTags) != 1 || drift.MissingTags[0] != "beta" {
		t.Fatalf("expected beta missing, got %+v", drift.MissingTags)
	}
	// The rank-1 lens still surfaces the uncovered-concept issue as low_r2.
	if byID["novel"].Reason != "low_r2" {
		t.Fatalf("expected novel issue as low_r2, got %q", byID["novel"].Reason)
	}
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

	det := NewDetector(store, store, fakeExplorer{}, fw, nil, nil)
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

	det := NewDetector(store, store, fakeExplorer{}, nil, nil, nil)
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
