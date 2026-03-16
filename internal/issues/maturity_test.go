package issues

import "testing"

func TestDeriveIssueLifecycleMetricsAddsMaturityAndCounts(t *testing.T) {
	metrics := DeriveIssueLifecycleMetrics(
		"Safari export fails after tapping Share twice.\n1. Open the Share sheet.\n2. Tap Export as PDF.\nError: WebKit exception in /app/export/pdf.ts:42.",
		[]IssuePost{
			{Sequence: 1, Raw: "Initial report"},
			{Sequence: 2, Kind: "refinement", Raw: "Add exact repro steps"},
			{Sequence: 3, Kind: "progress", Raw: "Added logging around export"},
		},
		[]IssueSnapshot{
			{Sequence: 1, Embedding: []float64{1, 0, 0}, TagScores: []TagRelevance{{Tag: "export", Relevance: 0.8}}},
			{Sequence: 2, Embedding: []float64{0.98, 0.02, 0}, TagScores: []TagRelevance{{Tag: "export", Relevance: 0.82}}},
		},
	)

	if metrics == nil {
		t.Fatal("expected non-nil lifecycle metrics")
	}
	if metrics.Maturity == nil {
		t.Fatalf("expected maturity to be computed, got %#v", metrics)
	}
	if metrics.RefinementCount != 1 || metrics.ProgressCount != 1 {
		t.Fatalf("expected counts 1/1, got %#v", metrics)
	}
	if *metrics.Maturity <= 0 {
		t.Fatalf("expected positive maturity, got %#v", metrics)
	}
}

func TestIssueMaturityPrefersStableCuratedIssues(t *testing.T) {
	stableCurated := DeriveIssueLifecycleMetrics(
		"Safari export fails after tapping Share twice.\n1. Open Share.\n2. Tap Export as PDF.\n3. Observe timeout.\nError: WebKit exception in /app/export/pdf.ts:42.",
		[]IssuePost{
			{Sequence: 1, Raw: "Initial report"},
			{Sequence: 2, Kind: "refinement", Raw: "Add repro"},
			{Sequence: 3, Kind: "refinement", Raw: "Clarify expected behavior"},
			{Sequence: 4, Kind: "progress", Raw: "Added logging"},
		},
		[]IssueSnapshot{
			{Sequence: 1, Embedding: []float64{1, 0, 0}, TagScores: []TagRelevance{{Tag: "export", Relevance: 0.8}}},
			{Sequence: 2, Embedding: []float64{0.98, 0.02, 0}, TagScores: []TagRelevance{{Tag: "export", Relevance: 0.82}}},
			{Sequence: 3, Embedding: []float64{0.97, 0.03, 0}, TagScores: []TagRelevance{{Tag: "export", Relevance: 0.83}}},
		},
	)

	churnyActive := DeriveIssueLifecycleMetrics(
		"Export issue",
		[]IssuePost{
			{Sequence: 1, Raw: "Initial report"},
			{Sequence: 2, Kind: "refinement", Raw: "Actually this may be auth related"},
			{Sequence: 3, Kind: "refinement", Raw: "No, maybe export pipeline"},
			{Sequence: 4, Kind: "progress", Raw: "Investigated callback flow"},
		},
		[]IssueSnapshot{
			{Sequence: 1, Embedding: []float64{1, 0, 0}, TagScores: []TagRelevance{{Tag: "export", Relevance: 0.8}}},
			{Sequence: 2, Embedding: []float64{0, 1, 0}, TagScores: []TagRelevance{{Tag: "auth", Relevance: 0.9}}},
			{Sequence: 3, Embedding: []float64{0.5, 0.5, 0}, TagScores: []TagRelevance{{Tag: "export", Relevance: 0.5}, {Tag: "auth", Relevance: 0.5}}},
		},
	)

	if stableCurated == nil || stableCurated.Maturity == nil || churnyActive == nil || churnyActive.Maturity == nil {
		t.Fatalf("expected maturity metrics, got stable=%#v churny=%#v", stableCurated, churnyActive)
	}
	if *stableCurated.Maturity <= *churnyActive.Maturity {
		t.Fatalf("expected stable curated maturity %v > churny maturity %v", *stableCurated.Maturity, *churnyActive.Maturity)
	}
}
