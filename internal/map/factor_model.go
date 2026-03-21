package issuemap

import (
	"math"

	"gonum.org/v1/gonum/mat"

	"splat/internal/issues"
	"splat/internal/scoring"
	"splat/internal/vectors"
)

// FactorDecomposition holds the results of decomposing issue embeddings into
// factor-predicted and residual components. The weights are data-driven,
// computed from the variance explained by the tag-factor model.
type FactorDecomposition struct {
	FactorEmbeddings   map[string][]float64 // factor-predicted embedding per issue
	ResidualEmbeddings map[string][]float64 // residual embedding per issue
	FactorWeight       float64              // data-driven weight for factor similarity
	ResidualWeight     float64              // data-driven weight for residual similarity
	IssueR2            map[string]float64   // per-issue R² (variance explained by factors)
	AggregateR2        float64              // mean R² across all decomposed issues
}

// ComputeFactorDecomposition projects each issue embedding onto the direction
// predicted by its tag loadings through the tag covariance matrix, then
// splits the embedding into a factor-predicted component and a residual.
// The blend weights are determined by the variance each component explains.
func ComputeFactorDecomposition(
	items []issues.Issue,
	tagNames []string,
	issueEmbeddings map[string][]float64,
	tagEmbeddings map[string][]float64,
) FactorDecomposition {
	decomp := FactorDecomposition{
		FactorEmbeddings:   make(map[string][]float64, len(items)),
		ResidualEmbeddings: make(map[string][]float64, len(items)),
		IssueR2:            make(map[string]float64, len(items)),
		FactorWeight:       scoring.FactorWeight,
		ResidualWeight:     scoring.SemanticWeight,
	}

	if len(items) < scoring.MinDecompositionIssues || len(tagNames) == 0 {
		return decomp
	}

	tagIndex := make(map[string]int, len(tagNames))
	for i, tag := range tagNames {
		tagIndex[tag] = i
	}

	tagCov := buildTagCovariance(tagNames, tagEmbeddings)

	// Determine expected embedding dimension from tag embeddings.
	embDim := 0
	for _, emb := range tagEmbeddings {
		if len(emb) > 0 {
			embDim = len(emb)
			break
		}
	}
	if embDim == 0 {
		return decomp
	}

	var varFactor, varResidual float64
	validCount := 0

	for _, item := range items {
		issueEmb := issueEmbeddings[item.ID]
		if len(issueEmb) == 0 || len(issueEmb) != embDim {
			// Dimension mismatch or missing embedding — skip decomposition.
			continue
		}

		totalVar := dotProduct(issueEmb, issueEmb)

		factorEmb := synthesizeFactorEmbedding(item.TagScores, tagIndex, tagCov, tagEmbeddings, tagNames, len(tagNames), embDim)
		if isZeroVector(factorEmb) {
			// No tags — full weight to residual for this issue.
			decomp.ResidualEmbeddings[item.ID] = append([]float64(nil), issueEmb...)
			decomp.FactorEmbeddings[item.ID] = make([]float64, embDim)
			decomp.IssueR2[item.ID] = 0
			varResidual += totalVar
			validCount++
			continue
		}
		normalizeVector(factorEmb)

		// Project issue embedding onto factor-predicted direction.
		projScalar := dotProduct(issueEmb, factorEmb)
		proj := make([]float64, embDim)
		residual := make([]float64, embDim)
		for d := range embDim {
			proj[d] = projScalar * factorEmb[d]
			residual[d] = issueEmb[d] - proj[d]
		}

		projVar := dotProduct(proj, proj)
		resVar := dotProduct(residual, residual)
		varFactor += projVar
		varResidual += resVar

		// R²_issue = 1 - var(residual) / var(total)
		if totalVar > 0 {
			decomp.IssueR2[item.ID] = 1 - resVar/totalVar
		}

		// Normalize for similarity computation.
		if !isZeroVector(proj) {
			normalizeVector(proj)
		}
		if !isZeroVector(residual) {
			normalizeVector(residual)
		}

		decomp.FactorEmbeddings[item.ID] = proj
		decomp.ResidualEmbeddings[item.ID] = residual
		validCount++
	}

	if validCount < scoring.MinDecompositionIssues {
		return decomp
	}

	// Compute data-driven weights from variance statistics.
	varFactor /= float64(validCount)
	varResidual /= float64(validCount)
	aggTotalVar := varFactor + varResidual
	if aggTotalVar > 0 {
		fw := varFactor / aggTotalVar
		decomp.FactorWeight = clamp(fw, scoring.MinFactorWeight, scoring.MaxFactorWeight)
		decomp.ResidualWeight = 1 - decomp.FactorWeight
		decomp.AggregateR2 = varFactor / aggTotalVar
	}

	return decomp
}

// DecomposeEmbedding performs the same factor/residual split for a single
// embedding vector (e.g. a search query).
func DecomposeEmbedding(
	embedding []float64,
	tagScores []issues.TagRelevance,
	tagNames []string,
	tagEmbeddings map[string][]float64,
	tagCov *mat.Dense,
) (factor, residual []float64) {
	embDim := len(embedding)
	if embDim == 0 || len(tagNames) == 0 {
		return make([]float64, embDim), append([]float64(nil), embedding...)
	}

	tagIndex := make(map[string]int, len(tagNames))
	for i, tag := range tagNames {
		tagIndex[tag] = i
	}

	factorEmb := synthesizeFactorEmbedding(tagScores, tagIndex, tagCov, tagEmbeddings, tagNames, len(tagNames), embDim)
	if isZeroVector(factorEmb) {
		return make([]float64, embDim), append([]float64(nil), embedding...)
	}
	normalizeVector(factorEmb)

	projScalar := dotProduct(embedding, factorEmb)
	factor = make([]float64, embDim)
	residual = make([]float64, embDim)
	for d := range embDim {
		factor[d] = projScalar * factorEmb[d]
		residual[d] = embedding[d] - factor[d]
	}

	if !isZeroVector(factor) {
		normalizeVector(factor)
	}
	if !isZeroVector(residual) {
		normalizeVector(residual)
	}

	return factor, residual
}

// BlendFromDecomposition computes the blended similarity using the
// decomposition's data-driven weights.
func BlendFromDecomposition(decomp FactorDecomposition, factorA, residualA, factorB, residualB []float64) (factorSim, residualSim, blended float64) {
	factorSim = vectors.UnitCosineSimilarity(factorA, factorB)
	residualSim = vectors.UnitCosineSimilarity(residualA, residualB)
	blended = decomp.FactorWeight*factorSim + decomp.ResidualWeight*residualSim
	return factorSim, residualSim, blended
}

// synthesizeFactorEmbedding builds the factor-predicted embedding for an issue
// by weighting tag embeddings through the tag covariance matrix.
func synthesizeFactorEmbedding(
	tagScores []issues.TagRelevance,
	tagIndex map[string]int,
	tagCov *mat.Dense,
	tagEmbeddings map[string][]float64,
	tagsByIndex []string,
	numTags, embDim int,
) []float64 {
	// Build tag loading vector r_i.
	baseData := make([]float64, numTags)
	hasTags := false
	for _, ts := range tagScores {
		if idx, ok := tagIndex[ts.Tag]; ok {
			baseData[idx] = ts.Relevance
			hasTags = true
		}
	}
	if !hasTags {
		return make([]float64, embDim)
	}

	// Transform through covariance: w = r_i × Σ_tags.
	r := mat.NewVecDense(numTags, baseData)
	var w mat.VecDense
	w.MulVec(tagCov.T(), r)

	// Build tag embedding matrix T (numTags × embDim) and compute ê = T^T × w.
	tagMat := mat.NewDense(numTags, embDim, nil)
	for k := range numTags {
		tagEmb := tagEmbeddings[tagsByIndex[k]]
		if len(tagEmb) == embDim {
			tagMat.SetRow(k, tagEmb)
		}
	}

	var result mat.VecDense
	result.MulVec(tagMat.T(), &w)

	factorEmb := make([]float64, embDim)
	for d := range embDim {
		factorEmb[d] = result.AtVec(d)
	}
	return factorEmb
}

func dotProduct(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
