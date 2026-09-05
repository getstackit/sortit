package main

import (
	"strings"
	"testing"

	"sortit/internal/matheval"
)

func TestRenderComparisonStatesRunCountAndRankingBaseline(t *testing.T) {
	got := renderComparison(matheval.TagEvalArtifact{
		Reports: []matheval.TagEvalReport{{Model: "gpt-test", Runs: 2}},
	})
	for _, want := range []string{
		"across 2 serial model runs",
		"fixture's ground-truth tag scores",
		"between-model comparison",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered comparison missing %q:\n%s", want, got)
		}
	}
}
