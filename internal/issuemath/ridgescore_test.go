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

func TestComputeRidgeScoresHighLambdaRecoversAnchor(t *testing.T) {
	// λ → ∞: the prior term dominates and f ≈ r even when the embedding
	// disagrees with the anchor.
	tagEmbeddings := [][]float64{
		{1, 0},
		{0, 1},
	}
	issueEmbedding := []float64{1, 0}
	anchor := []float64{-0.4, 0.9}

	out := ComputeRidgeScores(tagEmbeddings, issueEmbedding, anchor, 1e6)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	for i := range anchor {
		if math.Abs(out[i]-anchor[i]) > 1e-4 {
			t.Fatalf("huge lambda should recover anchor; got %+v, want %+v", out, anchor)
		}
	}
}

func TestComputeRidgeScoresLowLambdaRecoversLeastSquares(t *testing.T) {
	// λ → 0: the anchor is ignored and f solves min ||e − Tᵀf||².
	// With orthonormal tag rows the least-squares solution is f = T e.
	tagEmbeddings := [][]float64{
		{1, 0, 0},
		{0, 1, 0},
	}
	issueEmbedding := []float64{0.6, 0.8, 0}
	anchor := []float64{-1, -1} // deliberately far from the LS solution

	out := ComputeRidgeScores(tagEmbeddings, issueEmbedding, anchor, 1e-9)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	want := []float64{0.6, 0.8}
	for i := range want {
		if math.Abs(out[i]-want[i]) > 1e-4 {
			t.Fatalf("tiny lambda should recover least squares; got %+v, want %+v", out, want)
		}
	}
}

func TestComputeRidgeScoresDiagonalMatchesScalarWrapper(t *testing.T) {
	tagEmbeddings := [][]float64{
		{0.8, 0.6},
		{0.6, -0.8},
	}
	issueEmbedding := []float64{1, 0}
	anchor := []float64{0.5, 0.2}

	scalar := ComputeRidgeScores(tagEmbeddings, issueEmbedding, anchor, 0.5)
	diagonal := ComputeRidgeScoresDiagonal(tagEmbeddings, issueEmbedding, anchor, []float64{0.5, 0.5})
	if scalar == nil || diagonal == nil {
		t.Fatal("expected non-nil results")
	}
	for i := range scalar {
		if math.Abs(scalar[i]-diagonal[i]) > 1e-12 {
			t.Fatalf("uniform diagonal should match scalar wrapper; got %+v vs %+v", diagonal, scalar)
		}
	}
}

func TestComputeRidgeScoresDiagonalFreesWeaklyAnchoredTags(t *testing.T) {
	// Both tags are anchored at 0 and the embedding supports both equally.
	// The weakly penalized tag should move much further from its anchor
	// toward the least-squares fit than the strongly penalized one.
	tagEmbeddings := [][]float64{
		{1, 0},
		{0, 1},
	}
	norm := 1 / math.Sqrt2
	issueEmbedding := []float64{norm, norm}
	anchor := []float64{0, 0}
	lambdas := []float64{0.5, 0.05}

	out := ComputeRidgeScoresDiagonal(tagEmbeddings, issueEmbedding, anchor, lambdas)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	strongMove := math.Abs(out[0] - anchor[0])
	weakMove := math.Abs(out[1] - anchor[1])
	if weakMove <= strongMove {
		t.Fatalf("weakly anchored tag should move more freely; strong moved %v, weak moved %v", strongMove, weakMove)
	}
	// With orthonormal rows the solve decouples: f_k = (T e)_k / (1 + λ_k).
	wantStrong := norm / 1.5
	wantWeak := norm / 1.05
	if math.Abs(out[0]-wantStrong) > 1e-9 || math.Abs(out[1]-wantWeak) > 1e-9 {
		t.Fatalf("expected decoupled solutions [%v %v]; got %+v", wantStrong, wantWeak, out)
	}
}

func TestComputeRidgeScoresDiagonalRejectsMismatchedLambdas(t *testing.T) {
	got := ComputeRidgeScoresDiagonal(
		[][]float64{{1, 0}, {0, 1}},
		[]float64{1, 0},
		[]float64{0, 0},
		[]float64{0.5}, // wrong length
	)
	if got != nil {
		t.Fatalf("expected nil for lambdas length mismatch; got %+v", got)
	}
}

func TestDriftCosineIsOneForProportionalVectors(t *testing.T) {
	// Uniform shrinkage (f = c·r, c > 0) must not register as drift.
	r := []float64{0.9, -0.3, 0, 0.5}
	f := []float64{0.45, -0.15, 0, 0.25}
	if got := DriftCosine(f, r); math.Abs(got-1) > 1e-12 {
		t.Fatalf("DriftCosine(f, r) with f ∝ r = %v, want 1", got)
	}
}

func TestDriftCosineDetectsDirectionalDisagreement(t *testing.T) {
	r := []float64{1, 0}
	f := []float64{0, 1}
	if got := DriftCosine(f, r); math.Abs(got) > 1e-12 {
		t.Fatalf("orthogonal vectors should have zero drift cosine; got %v", got)
	}
	opposed := DriftCosine([]float64{-1, 0}, r)
	if math.Abs(opposed-(-1)) > 1e-12 {
		t.Fatalf("opposed vectors should have drift cosine -1; got %v", opposed)
	}
}

func TestDriftCosineDegenerateInputs(t *testing.T) {
	if got := DriftCosine([]float64{1, 0}, []float64{0, 0}); got != 0 {
		t.Fatalf("zero anchor should return 0; got %v", got)
	}
	if got := DriftCosine([]float64{1, 0}, []float64{1}); got != 0 {
		t.Fatalf("mismatched shapes should return 0; got %v", got)
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
