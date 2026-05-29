package issuemath

import (
	"gonum.org/v1/gonum/mat"
)

// ComputeRidgeScores returns the anchored-ridge refined tag-relevance
// vector for one issue. This is the consumer-side realization of the
// signed-loadings math from docs/math-evolution.md:
//
//	f = (T Tᵀ + λI)⁻¹ (T e + λ r)
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
//   - λ ∈ [0, ∞) controls shrinkage. λ = 0 ignores the anchor (pure
//     embedding-driven); λ → ∞ returns the anchor unchanged.
//
// Output dimensions match `r`. The function returns nil when the
// inputs disagree on shape or the system is singular (degenerate
// tag-embedding matrix).
func ComputeRidgeScores(
	tagEmbeddings [][]float64,
	issueEmbedding []float64,
	anchor []float64,
	lambda float64,
) []float64 {
	tagCount := len(tagEmbeddings)
	if tagCount == 0 || len(anchor) != tagCount {
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

	// Add λI to the diagonal.
	for i := range tagCount {
		gram.Set(i, i, gram.At(i, i)+lambda)
	}

	// projection = T e — tagCount-length vector.
	eVec := mat.NewVecDense(dim, issueEmbedding)
	var projection mat.VecDense
	projection.MulVec(tMat, eVec)

	// rhs = T e + λ r
	rhs := mat.NewVecDense(tagCount, nil)
	for i := range tagCount {
		rhs.SetVec(i, projection.AtVec(i)+lambda*anchor[i])
	}

	// Solve (T Tᵀ + λI) f = rhs.
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
