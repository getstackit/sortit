package issuemath

import (
	"math"

	"gonum.org/v1/gonum/mat"

	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/vectors"
)

// FactorDecomposition holds the results of decomposing issue embeddings into
// factor-predicted and residual components. The weights are data-driven,
// computed from the variance explained by the tag-factor model.
//
// Embeddings and R² values are stored in parallel slices indexed by a single
// map, avoiding per-field hash table overhead.
type FactorDecomposition struct {
	used          bool
	index         map[string]int // issue ID → position in parallel slices
	factors       [][]float64    // factor-predicted embedding per issue
	residuals     [][]float64    // residual embedding per issue
	factorNorms   []float64      // pre-normalization explained magnitude
	residualNorms []float64      // pre-normalization unexplained magnitude
	r2s           []float64      // per-issue R² (variance explained by factors)

	FactorWeight   float64 // data-driven weight for factor similarity
	ResidualWeight float64 // data-driven weight for residual similarity
	AggregateR2    float64 // mean R² across all decomposed issues
}

func newFactorDecomposition(capacity int) FactorDecomposition {
	return FactorDecomposition{
		index:          make(map[string]int, capacity),
		factors:        make([][]float64, 0, capacity),
		residuals:      make([][]float64, 0, capacity),
		factorNorms:    make([]float64, 0, capacity),
		residualNorms:  make([]float64, 0, capacity),
		r2s:            make([]float64, 0, capacity),
		FactorWeight:   scoring.FactorWeight,
		ResidualWeight: scoring.SemanticWeight,
	}
}

// FactorEmbedding returns the factor-predicted embedding for the given issue, or nil.
func (d FactorDecomposition) FactorEmbedding(id string) []float64 {
	if !d.used {
		return nil
	}
	if i, ok := d.index[id]; ok {
		return d.factors[i]
	}
	return nil
}

// ResidualEmbedding returns the residual embedding for the given issue, or nil.
func (d FactorDecomposition) ResidualEmbedding(id string) []float64 {
	if !d.used {
		return nil
	}
	if i, ok := d.index[id]; ok {
		return d.residuals[i]
	}
	return nil
}

// FactorNorm returns the pre-normalization explained magnitude for the issue.
func (d FactorDecomposition) FactorNorm(id string) (float64, bool) {
	if !d.used {
		return 0, false
	}
	if i, ok := d.index[id]; ok {
		return d.factorNorms[i], true
	}
	return 0, false
}

// ResidualNorm returns the pre-normalization unexplained magnitude for the issue.
func (d FactorDecomposition) ResidualNorm(id string) (float64, bool) {
	if !d.used {
		return 0, false
	}
	if i, ok := d.index[id]; ok {
		return d.residualNorms[i], true
	}
	return 0, false
}

// IssueR2 returns the R² value for the given issue and whether it was decomposed.
func (d FactorDecomposition) IssueR2(id string) (float64, bool) {
	if !d.used {
		return 0, false
	}
	if i, ok := d.index[id]; ok {
		return d.r2s[i], true
	}
	return 0, false
}

// DecomposedCount returns the number of issues that were decomposed.
func (d FactorDecomposition) DecomposedCount() int {
	if !d.used {
		return 0
	}
	return len(d.index)
}

// Decomposed returns true if any issues were decomposed.
func (d FactorDecomposition) Decomposed() bool {
	return d.used
}

// AllR2 iterates over all decomposed issues and their R² values.
func (d FactorDecomposition) AllR2(fn func(id string, r2 float64)) {
	if !d.used {
		return
	}
	for id, i := range d.index {
		fn(id, d.r2s[i])
	}
}

// put appends a decomposed issue to the parallel slices.
func (d *FactorDecomposition) put(id string, factor, residual []float64, factorNorm, residualNorm, r2 float64) {
	idx := len(d.factors)
	d.index[id] = idx
	d.factors = append(d.factors, factor)
	d.residuals = append(d.residuals, residual)
	d.factorNorms = append(d.factorNorms, factorNorm)
	d.residualNorms = append(d.residualNorms, residualNorm)
	d.r2s = append(d.r2s, r2)
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
	fallback := newFactorDecomposition(len(items))

	if len(items) < scoring.MinDecompositionIssues || len(tagNames) == 0 {
		return fallback
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
		return fallback
	}

	decomp := newFactorDecomposition(len(items))
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
			residual := append([]float64(nil), issueEmb...)
			residualNorm := math.Sqrt(totalVar)
			if !isZeroVector(residual) {
				normalizeVector(residual)
			}
			decomp.put(item.ID, make([]float64, embDim), residual, 0, residualNorm, 0)
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
		projNorm := math.Sqrt(projVar)
		residualNorm := math.Sqrt(resVar)
		varFactor += projVar
		varResidual += resVar

		// R²_issue = 1 - var(residual) / var(total)
		r2 := 0.0
		if totalVar > 0 {
			r2 = 1 - resVar/totalVar
		}

		// Normalize for similarity computation.
		if !isZeroVector(proj) {
			normalizeVector(proj)
		}
		if !isZeroVector(residual) {
			normalizeVector(residual)
		}

		decomp.put(item.ID, proj, residual, projNorm, residualNorm, r2)
		validCount++
	}

	if validCount < scoring.MinDecompositionIssues {
		return fallback
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
	decomp.used = true

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
		residual = append([]float64(nil), embedding...)
		if !isZeroVector(residual) {
			normalizeVector(residual)
		}
		return make([]float64, embDim), residual
	}

	tagIndex := make(map[string]int, len(tagNames))
	for i, tag := range tagNames {
		tagIndex[tag] = i
	}

	factorEmb := synthesizeFactorEmbedding(tagScores, tagIndex, tagCov, tagEmbeddings, tagNames, len(tagNames), embDim)
	if isZeroVector(factorEmb) {
		residual = append([]float64(nil), embedding...)
		if !isZeroVector(residual) {
			normalizeVector(residual)
		}
		return make([]float64, embDim), residual
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
