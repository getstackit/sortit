package matheval

import (
	"math"
	"testing"
)

func TestNDCGAtK_PerfectRanking(t *testing.T) {
	grades := map[string]int{"a": 3, "b": 2, "c": 1}
	if got := NDCGAtK([]string{"a", "b", "c"}, grades, 8); math.Abs(got-1) > 1e-9 {
		t.Errorf("perfect ranking NDCG = %f, want 1", got)
	}
}

func TestNDCGAtK_ReversedRankingIsWorse(t *testing.T) {
	grades := map[string]int{"a": 3, "b": 2, "c": 1}
	perfect := NDCGAtK([]string{"a", "b", "c"}, grades, 8)
	reversed := NDCGAtK([]string{"c", "b", "a"}, grades, 8)
	if reversed >= perfect {
		t.Errorf("reversed NDCG %f should be below perfect %f", reversed, perfect)
	}
	if reversed <= 0 {
		t.Errorf("reversed NDCG %f should still be positive", reversed)
	}
}

func TestNDCGAtK_IrrelevantResultsScoreZero(t *testing.T) {
	grades := map[string]int{"a": 3}
	if got := NDCGAtK([]string{"x", "y", "z"}, grades, 8); got != 0 {
		t.Errorf("NDCG = %f, want 0 for all-irrelevant ranking", got)
	}
}

func TestNDCGAtK_TruncatesAtK(t *testing.T) {
	grades := map[string]int{"a": 3}
	// Relevant result at position 3 with k=2 contributes nothing.
	if got := NDCGAtK([]string{"x", "y", "a"}, grades, 2); got != 0 {
		t.Errorf("NDCG@2 = %f, want 0 when the only relevant hit is at rank 3", got)
	}
}

func TestRecallAtK(t *testing.T) {
	grades := map[string]int{"a": 3, "b": 1, "c": 2, "ignored": 0}
	if got := RecallAtK([]string{"a", "x", "c"}, grades, 8); math.Abs(got-2.0/3.0) > 1e-9 {
		t.Errorf("Recall = %f, want 2/3", got)
	}
	if got := RecallAtK([]string{"a", "x", "c"}, grades, 1); math.Abs(got-1.0/3.0) > 1e-9 {
		t.Errorf("Recall@1 = %f, want 1/3", got)
	}
	if got := RecallAtK([]string{"x"}, map[string]int{}, 8); got != 0 {
		t.Errorf("Recall = %f, want 0 with no relevant issues", got)
	}
}

func TestSummarize(t *testing.T) {
	dist := Summarize([]float64{0.1, 0.2, 0.3, 0.4, 0.5})
	if dist.Count != 5 {
		t.Errorf("Count = %d, want 5", dist.Count)
	}
	if math.Abs(dist.Mean-0.3) > 1e-9 {
		t.Errorf("Mean = %f, want 0.3", dist.Mean)
	}
	if math.Abs(dist.Median-0.3) > 1e-9 {
		t.Errorf("Median = %f, want 0.3", dist.Median)
	}
	// Linear interpolation: p10 of 5 points sits at index 0.4 → 0.14.
	if math.Abs(dist.P10-0.14) > 1e-9 {
		t.Errorf("P10 = %f, want 0.14", dist.P10)
	}
	if math.Abs(dist.P90-0.46) > 1e-9 {
		t.Errorf("P90 = %f, want 0.46", dist.P90)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	if dist := Summarize(nil); dist != (Distribution{}) {
		t.Errorf("Summarize(nil) = %+v, want zero value", dist)
	}
}

func TestNoiseVecIsDeterministicAndUnit(t *testing.T) {
	a := noiseVec("seed")
	b := noiseVec("seed")
	other := noiseVec("other")
	var dotSame, dotOther, norm float64
	for i := range a {
		dotSame += a[i] * b[i]
		dotOther += a[i] * other[i]
		norm += a[i] * a[i]
	}
	if math.Abs(norm-1) > 1e-9 {
		t.Errorf("noiseVec norm² = %f, want 1", norm)
	}
	if math.Abs(dotSame-1) > 1e-9 {
		t.Error("noiseVec is not deterministic for the same seed")
	}
	if math.Abs(dotOther) > 0.6 {
		t.Errorf("different seeds should be roughly orthogonal, cos = %f", dotOther)
	}
}
