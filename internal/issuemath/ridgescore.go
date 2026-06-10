package issuemath

import (
	"math"

	"gonum.org/v1/gonum/mat"
)

// ComputeRidgeScores returns the anchored-ridge refined tag-relevance
// vector for one issue with a single shared shrinkage strength:
//
//	f = (T Tᵀ + λI)⁻¹ (T e + λ r)
//
// It is the uniform-penalty special case of ComputeRidgeScoresDiagonal;
// see that function for the meaning of each input. λ ∈ [0, ∞) controls
// shrinkage: λ = 0 ignores the anchor (pure embedding-driven); λ → ∞
// returns the anchor unchanged.
func ComputeRidgeScores(
	tagEmbeddings [][]float64,
	issueEmbedding []float64,
	anchor []float64,
	lambda float64,
) []float64 {
	lambdas := make([]float64, len(tagEmbeddings))
	for i := range lambdas {
		lambdas[i] = lambda
	}
	return ComputeRidgeScoresDiagonal(tagEmbeddings, issueEmbedding, anchor, lambdas)
}

// ComputeRidgeScoresDiagonal returns the anchored-ridge refined
// tag-relevance vector for one issue with a per-tag penalty. This is the
// consumer-side realization of the signed-loadings math from
// docs/math-evolution.md, generalized to a diagonal penalty matrix:
//
//	f = (T Tᵀ + Λ)⁻¹ (T e + Λ r)
//
// Where:
//   - T is the tag_count × dim matrix of tag embeddings.
//     Each row should be L2-normalized so T Tᵀ approximates the
//     tag-tag covariance.
//   - e is the dim-length issue embedding (also L2-normalized).
//   - r is the tag_count-length anchor vector. Typically built from
//     existing signed TagRelevance (positive relevance minus
//     negation magnitude), so the ridge "shrinks toward what the
//     analyzer already believes."
//   - Λ = diag(lambdas) holds one shrinkage strength per tag,
//     each in [0, ∞). λ_k = 0 lets tag k float freely to the
//     least-squares fit; λ_k → ∞ pins f_k to r_k. A per-tag Λ lets
//     callers anchor analyzer-scored tags strongly while leaving
//     tags without an analyzer opinion nearly free, so an anchor of
//     zero is treated as "no opinion" rather than evidence of absence.
//
// Output dimensions match `r`. The function returns nil when the
// inputs disagree on shape or the system is singular (degenerate
// tag-embedding matrix).
func ComputeRidgeScoresDiagonal(
	tagEmbeddings [][]float64,
	issueEmbedding []float64,
	anchor []float64,
	lambdas []float64,
) []float64 {
	tagCount := len(tagEmbeddings)
	if tagCount == 0 || len(anchor) != tagCount || len(lambdas) != tagCount {
		return nil
	}
	if len(issueEmbedding) == 0 {
		return nil
	}
	dim := len(issueEmbedding)
	for _, vec := range tagEmbeddings {
		if len(vec) != dim {
			return nil
		}
	}

	// Build T (tagCount × dim). Each row is one tag's embedding.
	tMat := mat.NewDense(tagCount, dim, nil)
	for i, vec := range tagEmbeddings {
		tMat.SetRow(i, vec)
	}

	// gram = T Tᵀ — a tagCount × tagCount tag-tag covariance.
	var gram mat.Dense
	gram.Mul(tMat, tMat.T())

	// Add Λ to the diagonal.
	for i := range tagCount {
		gram.Set(i, i, gram.At(i, i)+lambdas[i])
	}

	// projection = T e — tagCount-length vector.
	eVec := mat.NewVecDense(dim, issueEmbedding)
	var projection mat.VecDense
	projection.MulVec(tMat, eVec)

	// rhs = T e + Λ r
	rhs := mat.NewVecDense(tagCount, nil)
	for i := range tagCount {
		rhs.SetVec(i, projection.AtVec(i)+lambdas[i]*anchor[i])
	}

	// Solve (T Tᵀ + Λ) f = rhs.
	var f mat.VecDense
	if err := f.SolveVec(&gram, rhs); err != nil {
		return nil
	}

	out := make([]float64, tagCount)
	for i := range tagCount {
		out[i] = f.AtVec(i)
	}
	return out
}

// DriftCosine measures directional agreement between a refined ridge
// vector f and its anchor r as the cosine of the angle between them,
// restricted to components where either vector is non-zero.
//
// The Euclidean drift ‖f − r‖ from docs/math-evolution.md §5.4 is biased
// by uniform shrinkage: the least-squares term systematically pulls f's
// magnitude below r's, so distance mostly reports shrinkage rather than
// AI-vs-geometry disagreement. Cosine is scale-invariant: it stays 1.0
// when f is a uniformly shrunk copy of r and only drops when the two
// vectors point in genuinely different directions.
//
// Returns 0 when the shapes disagree or either restricted vector has
// zero norm (no overlap to compare).
func DriftCosine(f, r []float64) float64 {
	if len(f) != len(r) {
		return 0
	}
	var dot, fNormSq, rNormSq float64
	for i := range f {
		if f[i] == 0 && r[i] == 0 {
			continue
		}
		dot += f[i] * r[i]
		fNormSq += f[i] * f[i]
		rNormSq += r[i] * r[i]
	}
	if fNormSq == 0 || rNormSq == 0 {
		return 0
	}
	return dot / math.Sqrt(fNormSq*rNormSq)
}
