package issuemath

import (
	"math"

	"gonum.org/v1/gonum/mat"

	"sortit/internal/issues"
	"sortit/internal/scoring"
)

// DefaultRidgeLambdaGrid is the geometric search grid for GCV selection of
// the unscored-tag penalty. It spans three orders of magnitude around the
// debug-endpoint default so both the weak (missing-tag-friendly) and strong
// (overfitting-resistant) regimes are reachable.
var DefaultRidgeLambdaGrid = []float64{0.01, 0.03, 0.1, 0.3, 1.0, 3.0, 10.0}

// ridgeGCVSample is one issue's inputs to the GCV objective: its embedding,
// the signed anchor, and which tags the analyzer scored.
type ridgeGCVSample struct {
	embedding []float64
	anchor    []float64
	scored    []bool
}

// SelectRidgeLambdaGCV chooses the unscored-tag ridge penalty by minimizing
// generalized cross-validation error over the corpus, with the scored-tag
// penalty held fixed.
//
// Each issue is a ridge regression of its D-dimensional embedding e onto the
// K tag directions: ê = Tᵀf, f = (T Tᵀ + Λ)⁻¹(T e + Λ r). GCV estimates that
// regression's out-of-sample reconstruction error from the data alone — no
// labeled queries, no held-out split — as
//
//	GCV_i(λ) = ‖e_i − Tᵀf_i‖² / (D − df_i(λ))²,   df_i = tr((TTᵀ + Λ_i)⁻¹ TTᵀ)
//
// summed over issues. Minimizing it picks the λ that best trades fit against
// effective degrees of freedom, so the result adapts to the tag catalog's
// conditioning and the K/D ratio that drives overfitting — the reason a
// single hardcoded constant cannot transfer across corpora (a 16-tag/24-dim
// fixture and a 200-tag/1536-dim workspace want very different penalties).
//
// Λ_i is per-issue: scored tags get lambdaScored, the rest the candidate
// λ. Returns the GCV-optimal candidate, or false when the corpus is too
// small or the tag geometry is degenerate.
func SelectRidgeLambdaGCV(
	items []issues.Issue,
	tagNames []string,
	issueEmbeddings, tagEmbeddings map[string][]float64,
	lambdaScored float64,
	candidates []float64,
) (float64, bool) {
	if len(candidates) == 0 {
		candidates = DefaultRidgeLambdaGrid
	}
	if len(items) < scoring.MinDecompositionIssues || len(tagNames) == 0 {
		return 0, false
	}
	embDim := embeddingDim(tagEmbeddings)
	if embDim == 0 {
		return 0, false
	}

	numTags := len(tagNames)
	tagIndex := make(map[string]int, numTags)
	for i, t := range tagNames {
		tagIndex[t] = i
	}
	tagMatrix := ridgeTagMatrix(tagNames, tagEmbeddings, embDim)
	gram := ridgeGramMatrix(tagMatrix, numTags)

	samples := make([]ridgeGCVSample, 0, len(items))
	for _, item := range items {
		e := issueEmbeddings[item.ID]
		if len(e) != embDim || isZeroVector(e) {
			continue
		}
		anchor, scored := signedAnchor(item.TagScores, tagIndex, numTags)
		samples = append(samples, ridgeGCVSample{embedding: e, anchor: anchor, scored: scored})
	}
	if len(samples) < scoring.MinDecompositionIssues {
		return 0, false
	}

	bestLambda, bestGCV := 0.0, math.Inf(1)
	for _, lambda := range candidates {
		total, ok := aggregateRidgeGCV(tagMatrix, gram, samples, lambdaScored, lambda, embDim, numTags)
		if !ok {
			continue
		}
		if total < bestGCV {
			bestGCV, bestLambda = total, lambda
		}
	}
	if math.IsInf(bestGCV, 1) {
		return 0, false
	}
	return bestLambda, true
}

// aggregateRidgeGCV sums the per-issue GCV objective at one candidate
// unscored penalty. Returns false if any solve is singular or the effective
// degrees of freedom meet or exceed the embedding dimension (no residual
// degrees left — the GCV denominator would be non-positive).
func aggregateRidgeGCV(
	tagMatrix [][]float64,
	gram *mat.SymDense,
	samples []ridgeGCVSample,
	lambdaScored, lambdaUnscored float64,
	embDim, numTags int,
) (float64, bool) {
	var sum float64
	for _, s := range samples {
		// A = T Tᵀ + Λ (symmetric positive definite: gram is PSD, Λ > 0).
		a := mat.NewSymDense(numTags, nil)
		a.CopySym(gram)
		for k := range numTags {
			lambda := lambdaUnscored
			if s.scored[k] {
				lambda = lambdaScored
			}
			a.SetSym(k, k, a.At(k, k)+lambda)
		}

		var chol mat.Cholesky
		if !chol.Factorize(a) {
			return 0, false
		}

		// f = A⁻¹ (T e + Λ r).
		rhs := mat.NewVecDense(numTags, nil)
		for k := range numTags {
			lambda := lambdaUnscored
			if s.scored[k] {
				lambda = lambdaScored
			}
			rhs.SetVec(k, dotProduct(tagMatrix[k], s.embedding)+lambda*s.anchor[k])
		}
		var f mat.VecDense
		if err := chol.SolveVecTo(&f, rhs); err != nil {
			return 0, false
		}

		residSq := ridgeReconResidualSq(tagMatrix, &f, s.embedding, embDim, numTags)

		// Effective degrees of freedom df = tr(A⁻¹ T Tᵀ).
		var xinv mat.Dense
		if err := chol.SolveTo(&xinv, gram); err != nil {
			return 0, false
		}
		df := mat.Trace(&xinv)

		denom := float64(embDim) - df
		if denom <= 0 {
			return 0, false
		}
		sum += residSq / (denom * denom)
	}
	return sum, true
}

// ridgeGramMatrix builds T Tᵀ (the K×K tag-tag Gram matrix) as a symmetric
// dense matrix.
func ridgeGramMatrix(tagMatrix [][]float64, numTags int) *mat.SymDense {
	gram := mat.NewSymDense(numTags, nil)
	for i := range numTags {
		for j := i; j < numTags; j++ {
			gram.SetSym(i, j, dotProduct(tagMatrix[i], tagMatrix[j]))
		}
	}
	return gram
}

// ridgeReconResidualSq returns ‖e − Tᵀf‖².
func ridgeReconResidualSq(tagMatrix [][]float64, f mat.Vector, e []float64, embDim, numTags int) float64 {
	recon := make([]float64, embDim)
	for k := range numTags {
		fk := f.AtVec(k)
		if fk == 0 {
			continue
		}
		row := tagMatrix[k]
		for d := range embDim {
			recon[d] += fk * row[d]
		}
	}
	var sum float64
	for d := range embDim {
		diff := e[d] - recon[d]
		sum += diff * diff
	}
	return sum
}
