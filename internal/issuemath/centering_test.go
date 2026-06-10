package issuemath

import (
	"math"
	"testing"

	"sortit/internal/vectors"
)

func TestComputeCorpusMeans(t *testing.T) {
	t.Run("empty corpus yields zero means", func(t *testing.T) {
		means := ComputeCorpusMeans(nil, nil)
		if !means.IsZero() {
			t.Fatalf("means = %+v, want zero", means)
		}
	})

	t.Run("below minimum population yields nil means", func(t *testing.T) {
		means := ComputeCorpusMeans(
			map[string][]float64{"a": {1, 0}, "b": {0, 1}},
			map[string][]float64{"t": {2, 2}},
		)
		if !means.IsZero() {
			t.Fatalf("means = %+v, want zero for tiny populations", means)
		}
	})

	t.Run("issue and tag means are independent", func(t *testing.T) {
		means := ComputeCorpusMeans(
			map[string][]float64{
				"a": {1, 0}, "b": {0, 1}, "c": {1, 0}, "d": {0, 1}, "e": {1, 0}, "f": {0, 1},
			},
			map[string][]float64{
				"t1": {2, 2}, "t2": {2, 2}, "t3": {2, 2}, "t4": {2, 2}, "t5": {2, 2},
			},
		)
		assertVec(t, means.Issue, []float64{0.5, 0.5})
		assertVec(t, means.Tag, []float64{2, 2})
	})
}

func TestCenterEmbeddings(t *testing.T) {
	issueEmbeddings := map[string][]float64{
		"a": {1, 0}, "b": {0, 1}, "c": {1, 0}, "d": {0, 1}, "e": {1, 0}, "f": {0, 1},
	}
	tagEmbeddings := map[string][]float64{
		"t1": {1, 1}, "t2": {1, 0}, "t3": {1, 1}, "t4": {1, 0}, "t5": {1, 1}, "t6": {1, 0},
	}

	centeredIssues, centeredTags, means := CenterEmbeddings(issueEmbeddings, tagEmbeddings)

	// Inputs must not be mutated.
	assertVec(t, issueEmbeddings["a"], []float64{1, 0})
	assertVec(t, tagEmbeddings["t1"], []float64{1, 1})

	assertVec(t, means.Issue, []float64{0.5, 0.5})
	assertVec(t, centeredIssues["a"], []float64{1 / math.Sqrt2, -1 / math.Sqrt2})
	assertVec(t, centeredIssues["b"], []float64{-1 / math.Sqrt2, 1 / math.Sqrt2})

	// Centered opposing vectors are exactly anti-correlated.
	if sim := vectors.UnitCosineSimilarity(centeredIssues["a"], centeredIssues["b"]); math.Abs(sim-(-1)) > 1e-12 {
		t.Fatalf("centered cosine = %v, want -1", sim)
	}

	assertVec(t, means.Tag, []float64{1, 0.5})
	assertVec(t, centeredTags["t1"], []float64{0, 1})
	assertVec(t, centeredTags["t2"], []float64{0, -1})
}

func TestCenterEmbeddingsDegenerate(t *testing.T) {
	t.Run("single vector falls back to uncentered", func(t *testing.T) {
		centeredIssues, _, _ := CenterEmbeddings(map[string][]float64{"only": {3, 4}}, nil)
		assertVec(t, centeredIssues["only"], []float64{0.6, 0.8})
	})

	t.Run("zero vector stays zero", func(t *testing.T) {
		centeredIssues, _, _ := CenterEmbeddings(map[string][]float64{
			"zero": {0, 0},
			"a":    {1, 0}, "b": {0, 1}, "c": {1, 0}, "d": {0, 1},
		}, nil)
		assertVec(t, centeredIssues["zero"], []float64{0, 0})
	})

	t.Run("nil maps stay nil", func(t *testing.T) {
		centeredIssues, centeredTags := CenterEmbeddingsWith(CorpusMeans{}, nil, nil)
		if centeredIssues != nil || centeredTags != nil {
			t.Fatalf("got %v, %v, want nil maps", centeredIssues, centeredTags)
		}
	})

	t.Run("dimension mismatch falls back to uncentered", func(t *testing.T) {
		centeredIssues, _ := CenterEmbeddingsWith(
			CorpusMeans{Issue: []float64{1, 1, 1}},
			map[string][]float64{"a": {3, 4}},
			nil,
		)
		assertVec(t, centeredIssues["a"], []float64{0.6, 0.8})
	})
}

func TestCenterVector(t *testing.T) {
	got := CenterVector([]float64{1, 1}, []float64{1, 0})
	assertVec(t, got, []float64{0, 1})
}

func assertVec(t *testing.T, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
