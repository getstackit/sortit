package issuemath

import (
	"math"

	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"

	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/vectors"
)

// RidgeSimilarityMode selects which space the "factor" side of the ridge
// blend compares. The two are measured head-to-head in the matheval shadow
// harness before either becomes the ranking default.
type RidgeSimilarityMode int

const (
	// RidgeTagSpace compares the tag-space loadings cos(f_A, f_B). It is the
	// most interpretable: two issues are close when their refined tag
	// geometry agrees.
	RidgeTagSpace RidgeSimilarityMode = iota
	// RidgeReconSpace compares the embedding-space reconstructions
	// cos(Tᵀf_A, Tᵀf_B). It stays closest to today's rank-1 blend, which
	// also lives in embedding space.
	RidgeReconSpace
)

// RidgeVectors holds one embedding's anchored-ridge decomposition: the
// tag-space loading f, its reconstruction Tᵀf back in embedding space, and
// the residual e − Tᵀf. Direction vectors are unit-normalized for cosine
// similarity; the pre-normalization norms are retained for R² and
// diagnostics.
type RidgeVectors struct {
	Loading        []float64 // unit f (tag space)
	Reconstruction []float64 // unit Tᵀf (embedding space)
	Residual       []float64 // unit (e − Tᵀf) (embedding space)
	ReconNorm      float64   // ‖Tᵀf‖ before normalization
	ResidualNorm   float64   // ‖e − Tᵀf‖ before normalization
	R2             float64   // 1 − ‖residual‖²/‖e‖² (honest; may be < 0)
}

// RidgeDecomposition is the full-rank counterpart to FactorDecomposition.
// Where the rank-1 model projects each embedding onto a single
// tag-synthesized direction (so the embedding contributes only an alignment
// gate to factor similarity), the ridge model solves
//
//	f = (T Tᵀ + Λ)⁻¹ (T e + Λ r)
//
// per issue (docs/math-evolution.md §5), giving a T-dimensional loading
// whose reconstruction Tᵀf is a genuine least-squares-plus-anchor fit of the
// embedding. R² is therefore the textbook 1 − ‖e − Tᵀf‖²/‖e‖², not a rank-1
// squared cosine. The data-driven blend weights follow the same recipe as
// the rank-1 model: w_F is the aggregate R², clamped.
type RidgeDecomposition struct {
	used  bool
	index map[string]int
	vecs  []RidgeVectors

	FactorWeight   float64
	ResidualWeight float64
	AggregateR2    float64
}

// VectorsFor returns the stored decomposition for an issue.
func (d RidgeDecomposition) VectorsFor(id string) (RidgeVectors, bool) {
	if !d.used {
		return RidgeVectors{}, false
	}
	i, ok := d.index[id]
	if !ok {
		return RidgeVectors{}, false
	}
	return d.vecs[i], true
}

// Decomposed reports whether any issues were decomposed.
func (d RidgeDecomposition) Decomposed() bool { return d.used }

// AllR2 iterates over each decomposed issue's true R².
func (d RidgeDecomposition) AllR2(fn func(id string, r2 float64)) {
	if !d.used {
		return
	}
	for id, i := range d.index {
		fn(id, d.vecs[i].R2)
	}
}

// ComputeRidgeDecomposition runs the anchored ridge solve over a corpus.
// issueEmbeddings and tagEmbeddings are expected already corpus-mean
// centered, the same space the rank-1 decomposition consumes. lambdaScored
// is the per-tag penalty for tags the analyzer scored or negated;
// lambdaUnscored is the weaker penalty for catalog tags with no analyzer
// opinion, so their zero anchor reads as "no opinion" rather than evidence
// of absence (docs/math-evolution.md §5; scoring.RidgeAnchorLambda*).
func ComputeRidgeDecomposition(
	items []issues.Issue,
	tagNames []string,
	issueEmbeddings map[string][]float64,
	tagEmbeddings map[string][]float64,
	lambdaScored, lambdaUnscored float64,
) RidgeDecomposition {
	fallback := RidgeDecomposition{
		FactorWeight:   scoring.FactorWeight,
		ResidualWeight: scoring.SemanticWeight,
	}
	if len(items) < scoring.MinDecompositionIssues || len(tagNames) == 0 {
		return fallback
	}

	embDim := embeddingDim(tagEmbeddings)
	if embDim == 0 {
		return fallback
	}

	tagIndex := make(map[string]int, len(tagNames))
	for i, tag := range tagNames {
		tagIndex[tag] = i
	}
	tagMatrix := ridgeTagMatrix(tagNames, tagEmbeddings, embDim)
	solver := newRidgeSolver(tagMatrix, embDim)

	d := RidgeDecomposition{
		index:          make(map[string]int, len(items)),
		vecs:           make([]RidgeVectors, 0, len(items)),
		FactorWeight:   scoring.FactorWeight,
		ResidualWeight: scoring.SemanticWeight,
	}

	var sumResidualVar, sumTotalVar float64
	valid := 0
	for _, item := range items {
		e := issueEmbeddings[item.ID]
		if len(e) != embDim || vectors.IsZero(e) {
			continue
		}
		totalVar := dotProduct(e, e)

		rv, residualVar := ridgeVectorsFor(solver, e, item.TagScores, tagIndex, embDim, lambdaScored, lambdaUnscored, totalVar)
		d.put(item.ID, rv)
		sumResidualVar += residualVar
		sumTotalVar += totalVar
		valid++
	}

	if valid < scoring.MinDecompositionIssues {
		return fallback
	}

	if sumTotalVar > 0 {
		d.AggregateR2 = 1 - sumResidualVar/sumTotalVar
	}
	wF := clamp(d.AggregateR2, scoring.MinFactorWeight, scoring.MaxFactorWeight)
	d.FactorWeight = wF
	d.ResidualWeight = 1 - wF
	d.used = true
	return d
}

// DecomposeRidgeEmbedding decomposes a single external embedding (a search
// query or person profile) with the same solve as the corpus path.
func DecomposeRidgeEmbedding(
	embedding []float64,
	tagScores []issues.TagRelevance,
	tagNames []string,
	tagEmbeddings map[string][]float64,
	lambdaScored, lambdaUnscored float64,
) RidgeVectors {
	embDim := len(embedding)
	if embDim == 0 || len(tagNames) == 0 || vectors.IsZero(embedding) {
		return residualOnlyRidgeVectors(embedding, math.Sqrt(dotProduct(embedding, embedding)))
	}
	if embeddingDim(tagEmbeddings) != embDim {
		return residualOnlyRidgeVectors(embedding, math.Sqrt(dotProduct(embedding, embedding)))
	}

	tagIndex := make(map[string]int, len(tagNames))
	for i, tag := range tagNames {
		tagIndex[tag] = i
	}
	tagMatrix := ridgeTagMatrix(tagNames, tagEmbeddings, embDim)
	solver := newRidgeSolver(tagMatrix, embDim)

	rv, _ := ridgeVectorsFor(solver, embedding, tagScores, tagIndex, embDim, lambdaScored, lambdaUnscored, dotProduct(embedding, embedding))
	return rv
}

// RidgeBlend combines the ridge factor and residual similarities with the
// decomposition's data-driven weights. As with the rank-1 blend, a pair with
// no comparable evidence on one side marginalizes to the other so degenerate
// pairs stay on the same scale as fully decomposed ones.
func RidgeBlend(d RidgeDecomposition, a, b RidgeVectors, mode RidgeSimilarityMode) (factorSim, residualSim, blended float64) {
	switch mode {
	case RidgeReconSpace:
		factorSim = vectors.UnitCosineSimilarity(a.Reconstruction, b.Reconstruction)
	default:
		factorSim = vectors.UnitCosineSimilarity(a.Loading, b.Loading)
	}
	residualSim = vectors.UnitCosineSimilarity(a.Residual, b.Residual)

	wF, wR := d.FactorWeight, d.ResidualWeight
	noFactor := factorZero(a, mode) || factorZero(b, mode)
	noResidual := vectors.IsZero(a.Residual) || vectors.IsZero(b.Residual)
	switch {
	case noFactor && !noResidual:
		wF, wR = 0, 1
	case noResidual && !noFactor:
		wF, wR = 1, 0
	}

	blended = wF*factorSim + wR*residualSim
	return factorSim, residualSim, blended
}

func factorZero(v RidgeVectors, mode RidgeSimilarityMode) bool {
	if mode == RidgeReconSpace {
		return vectors.IsZero(v.Reconstruction)
	}
	return vectors.IsZero(v.Loading)
}

// ridgeSolver factors the per-issue anchored-ridge solve into a one-time
// setup and a cheap per-embedding solve. The tag matrix T and the base gram
// T Tᵀ are identical for every issue in a corpus (they depend only on the tag
// catalog), so building them once instead of per issue removes the dominant
// cost of a corpus decomposition — at production embedding dimensions T Tᵀ
// (O(K²·D)) dwarfs the per-issue Cholesky solve. The per-issue arithmetic is
// the same gonum operations on the same operands as the standalone
// ComputeRidgeScoresDiagonal, so the loadings are bit-for-bit identical.
type ridgeSolver struct {
	tagMatrix [][]float64
	dim       int
	tagCount  int

	tMat     *mat.Dense
	baseGram *mat.Dense

	// Scratch reused across solve calls. gram holds baseGram plus the
	// per-issue diagonal penalty; out holds the latest loadings and is
	// overwritten on the next solve, so callers that retain it must copy.
	gram       mat.Dense
	projection mat.VecDense
	rhs        *mat.VecDense
	f          mat.VecDense
	out        []float64
}

// newRidgeSolver precomputes T and T Tᵀ. Every row of tagMatrix must already
// be dim-length (ridgeTagMatrix guarantees this); zero rows for missing tag
// embeddings are fine and contribute nothing to the gram.
func newRidgeSolver(tagMatrix [][]float64, dim int) *ridgeSolver {
	tagCount := len(tagMatrix)
	tMat := mat.NewDense(tagCount, dim, nil)
	for i, vec := range tagMatrix {
		tMat.SetRow(i, vec)
	}
	var baseGram mat.Dense
	baseGram.Mul(tMat, tMat.T())
	return &ridgeSolver{
		tagMatrix: tagMatrix,
		dim:       dim,
		tagCount:  tagCount,
		tMat:      tMat,
		baseGram:  &baseGram,
		gram:      *mat.NewDense(tagCount, tagCount, nil),
		rhs:       mat.NewVecDense(tagCount, nil),
		out:       make([]float64, tagCount),
	}
}

// solve returns f = (T Tᵀ + diag(lambdas))⁻¹ (T e + diag(lambdas) r). The
// returned slice is solver-owned scratch, valid until the next solve call.
// Returns nil when the inputs disagree on shape or the system is singular.
func (s *ridgeSolver) solve(e, anchor, lambdas []float64) []float64 {
	if len(e) != s.dim || len(anchor) != s.tagCount || len(lambdas) != s.tagCount {
		return nil
	}

	// A = T Tᵀ + Λ (copy the shared base gram, then add the per-issue diagonal).
	s.gram.Copy(s.baseGram)
	for i := range s.tagCount {
		s.gram.Set(i, i, s.gram.At(i, i)+lambdas[i])
	}

	// projection = T e.
	eVec := mat.NewVecDense(s.dim, e)
	s.projection.MulVec(s.tMat, eVec)

	// rhs = T e + Λ r.
	for i := range s.tagCount {
		s.rhs.SetVec(i, s.projection.AtVec(i)+lambdas[i]*anchor[i])
	}

	if err := s.f.SolveVec(&s.gram, s.rhs); err != nil {
		return nil
	}
	for i := range s.tagCount {
		s.out[i] = s.f.AtVec(i)
	}
	return s.out
}

// ridgeVectorsFor solves the ridge system for one embedding and assembles
// its decomposition, returning the vectors and the residual variance.
func ridgeVectorsFor(
	solver *ridgeSolver,
	e []float64,
	tagScores []issues.TagRelevance,
	tagIndex map[string]int,
	embDim int,
	lambdaScored, lambdaUnscored, totalVar float64,
) (RidgeVectors, float64) {
	anchor, lambdas := ridgeAnchorAndLambdas(tagScores, tagIndex, solver.tagCount, lambdaScored, lambdaUnscored)
	f := solver.solve(e, anchor, lambdas)
	if f == nil || vectors.IsZero(f) {
		return residualOnlyRidgeVectors(e, math.Sqrt(totalVar)), totalVar
	}

	recon := reconstructEmbedding(f, solver.tagMatrix, embDim)
	if vectors.IsZero(recon) {
		return residualOnlyRidgeVectors(e, math.Sqrt(totalVar)), totalVar
	}
	residual := make([]float64, embDim)
	copy(residual, e)
	floats.Sub(residual, recon)

	reconNorm := math.Sqrt(dotProduct(recon, recon))
	residualVar := dotProduct(residual, residual)
	residualNorm := math.Sqrt(residualVar)

	r2 := 0.0
	if totalVar > 0 {
		r2 = 1 - residualVar/totalVar
	}

	loading := append([]float64(nil), f...)
	vectors.NormalizeUnit(loading)
	vectors.NormalizeUnit(recon)
	if !vectors.IsZero(residual) {
		vectors.NormalizeUnit(residual)
	}

	return RidgeVectors{
		Loading:        loading,
		Reconstruction: recon,
		Residual:       residual,
		ReconNorm:      reconNorm,
		ResidualNorm:   residualNorm,
		R2:             r2,
	}, residualVar
}

// residualOnlyRidgeVectors represents an embedding the ridge model could not
// explain at all: zero loading and reconstruction, the full (unit) embedding
// as residual, R² of zero.
func residualOnlyRidgeVectors(e []float64, totalNorm float64) RidgeVectors {
	residual := append([]float64(nil), e...)
	if !vectors.IsZero(residual) {
		vectors.NormalizeUnit(residual)
	}
	return RidgeVectors{
		Loading:        nil,
		Reconstruction: nil,
		Residual:       residual,
		ResidualNorm:   totalNorm,
	}
}

// signedAnchor builds the signed prior r = r⁺ − r⁻ and a mask of which tags
// the analyzer expressed an opinion on. Tag matching is by raw name,
// matching the rank-1 synthesizeFactorEmbedding path.
func signedAnchor(
	tagScores []issues.TagRelevance,
	tagIndex map[string]int,
	numTags int,
) (anchor []float64, scored []bool) {
	anchor = make([]float64, numTags)
	scored = make([]bool, numTags)
	for _, ts := range tagScores {
		idx, ok := tagIndex[ts.Tag]
		if !ok {
			continue
		}
		negation := 0.0
		if ts.Negation != nil {
			negation = *ts.Negation
		}
		anchor[idx] = ts.Relevance - negation
		scored[idx] = true
	}
	return anchor, scored
}

// ridgeAnchorAndLambdas builds the signed prior and the per-tag penalty
// vector. Tags the analyzer scored (or negated) get lambdaScored; the rest
// get lambdaUnscored.
func ridgeAnchorAndLambdas(
	tagScores []issues.TagRelevance,
	tagIndex map[string]int,
	numTags int,
	lambdaScored, lambdaUnscored float64,
) (anchor, lambdas []float64) {
	anchor, scored := signedAnchor(tagScores, tagIndex, numTags)
	lambdas = make([]float64, numTags)
	for i := range lambdas {
		if scored[i] {
			lambdas[i] = lambdaScored
		} else {
			lambdas[i] = lambdaUnscored
		}
	}
	return anchor, lambdas
}

// reconstructEmbedding computes Tᵀf — the linear combination of tag
// embeddings weighted by the ridge loadings. Each nonzero loading contributes
// an AXPY (recon += fk·row) through gonum's floats kernel.
func reconstructEmbedding(f []float64, tagMatrix [][]float64, embDim int) []float64 {
	recon := make([]float64, embDim)
	for k, fk := range f {
		if fk == 0 {
			continue
		}
		floats.AddScaled(recon, fk, tagMatrix[k])
	}
	return recon
}

// ridgeTagMatrix builds the tagCount × embDim matrix of tag embeddings in
// tagNames order. Missing embeddings become zero rows (they contribute
// nothing to the solve or reconstruction). ComputeRidgeScoresDiagonal
// requires every row to be embDim-length.
func ridgeTagMatrix(tagNames []string, tagEmbeddings map[string][]float64, embDim int) [][]float64 {
	matrix := make([][]float64, len(tagNames))
	for i, tag := range tagNames {
		emb := tagEmbeddings[tag]
		if len(emb) == embDim {
			matrix[i] = emb
		} else {
			matrix[i] = make([]float64, embDim)
		}
	}
	return matrix
}

func embeddingDim(tagEmbeddings map[string][]float64) int {
	for _, emb := range tagEmbeddings {
		if len(emb) > 0 {
			return len(emb)
		}
	}
	return 0
}

func (d *RidgeDecomposition) put(id string, v RidgeVectors) {
	d.index[id] = len(d.vecs)
	d.vecs = append(d.vecs, v)
}
