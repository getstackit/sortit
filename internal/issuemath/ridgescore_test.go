package issuemath

import (
	"math"
	"testing"
)

func TestComputeRidgeScoresShrinkagePullsTowardAnchor(t *testing.T) {
	// Two orthogonal tags, issue embedding aligned with tag 0.
	tagEmbeddings := [][]float64{
		{1, 0},
		{0, 1},
	}
	issueEmbedding := []float64{1, 0}
	anchor := []float64{0.0, 1.0}

	// High lambda → result close to anchor.
	high := ComputeRidgeScores(tagEmbeddings, issueEmbedding, anchor, 100.0)
	if high == nil {
		t.Fatal("expected non-nil result")
	}
	if math.Abs(high[0]-anchor[0]) > 0.05 || math.Abs(high[1]-anchor[1]) > 0.05 {
		t.Fatalf("high lambda should track anchor; got %+v, want ~%+v", high, anchor)
	}

	// Low lambda → result driven by the embedding projection (tag 0 dominant).
	low := ComputeRidgeScores(tagEmbeddings, issueEmbedding, anchor, 0.001)
	if low == nil {
		t.Fatal("expected non-nil result")
	}
	if low[0] < low[1] {
		t.Fatalf("low lambda should favor embedding-aligned tag; got %+v", low)
	}
}

func TestComputeRidgeScoresIdentityCase(t *testing.T) {
	// When tag embeddings are orthonormal and λ = 0, the system reduces to:
	//   f = (T Tᵀ)⁻¹ T e = T e   (since T Tᵀ = I for orthonormal rows)
	tagEmbeddings := [][]float64{
		{1, 0, 0},
		{0, 1, 0},
	}
	issueEmbedding := []float64{0.6, 0.8, 0}
	anchor := []float64{0, 0}

	out := ComputeRidgeScores(tagEmbeddings, issueEmbedding, anchor, 0.0)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if math.Abs(out[0]-0.6) > 0.01 || math.Abs(out[1]-0.8) > 0.01 {
		t.Fatalf("expected dot-product projection; got %+v", out)
	}
}

func TestComputeRidgeScoresRejectsMismatchedShapes(t *testing.T) {
	if got := ComputeRidgeScores(nil, []float64{1, 0}, []float64{1}, 0.1); got != nil {
		t.Fatalf("expected nil for empty tag embeddings; got %+v", got)
	}
	if got := ComputeRidgeScores(
		[][]float64{{1, 0}, {0, 1}},
		[]float64{1, 0},
		[]float64{1}, // wrong length
		0.1,
	); got != nil {
		t.Fatalf("expected nil for anchor length mismatch; got %+v", got)
	}
	if got := ComputeRidgeScores(
		[][]float64{{1, 0}, {0, 1, 0}}, // wrong row length
		[]float64{1, 0},
		[]float64{0, 0},
		0.1,
	); got != nil {
		t.Fatalf("expected nil for ragged tag rows; got %+v", got)
	}
}
