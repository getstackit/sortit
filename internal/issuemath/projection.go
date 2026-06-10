package issuemath

import (
	"fmt"
	"math"
	"slices"

	"gonum.org/v1/gonum/mat"

	"sortit/internal/issueanalytics"
	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/vectors"
)

type Position struct {
	X float64
	Y float64
}

const minProjectionIssueCount = 5

// minAlignmentSharedIssues is the minimum number of issues that must appear
// in both the previous and current layouts before Procrustes alignment is
// applied. Two points determine an orthogonal transform but degenerate too
// easily; below three shared anchors the deterministic loading-sign
// convention alone decides orientation.
const minAlignmentSharedIssues = 3

// ComputePositions lays out issues with no previous layout to align to:
// orientation falls back to the deterministic loading-sign convention.
// Callers that hold the previously rendered layout should prefer
// ComputePositionsAligned so the map keeps its orientation across rebuilds.
func ComputePositions(issues []issues.Issue, tags []string, tagEmbeddings map[string][]float64) (map[string]Position, error) {
	return ComputePositionsAligned(issues, tags, tagEmbeddings, nil)
}

// ComputePositionsAligned computes the PCA layout and then aligns it to
// `previous` — the positions of the last rendered layout (normalized
// [0.05, 0.95] space, possibly empty) — via orthogonal Procrustes, so PCA
// eigenvector sign flips and PC1/PC2 swaps between recomputes don't
// reorient the map users have built spatial memory of.
func ComputePositionsAligned(issues []issues.Issue, tags []string, tagEmbeddings map[string][]float64, previous map[string]Position) (map[string]Position, error) {
	n := len(issues)
	t := len(tags)
	if n == 0 {
		return map[string]Position{}, nil
	}
	if n < minProjectionIssueCount {
		return nil, fmt.Errorf("insufficient issues for projection: got %d, need at least %d", n, minProjectionIssueCount)
	}
	if t < 2 {
		return nil, fmt.Errorf("insufficient tag dimensions for projection: got %d, need at least 2", t)
	}

	tagIndex := make(map[string]int, t)
	for i, tag := range tags {
		tagIndex[tag] = i
	}

	// Build N×T relevance matrix
	data := make([]float64, n*t)
	for i, issue := range issues {
		for _, tr := range issue.TagScores {
			if j, ok := tagIndex[tr.Tag]; ok {
				data[i*t+j] = tr.Relevance
			}
		}
	}
	X := mat.NewDense(n, t, data)

	// Build T×T tag covariance matrix from tag embeddings (cosine similarity)
	tagCov := buildTagCovariance(tags, tagEmbeddings)

	// Transform: X' = X × Σ_tags — smears loadings across correlated tags
	var Xprime mat.Dense
	Xprime.Mul(X, tagCov)

	// Per-issue quality weights for projection stability.
	// Uses content confidence and maturity — NOT search-side boosts.
	weights := issueProjectionWeights(issues)

	// Weighted mean-center each column
	sumW := 0.0
	for _, w := range weights {
		sumW += w
	}
	for j := range t {
		wMean := 0.0
		for i := range n {
			wMean += weights[i] * Xprime.At(i, j)
		}
		wMean /= sumW
		for i := range n {
			Xprime.Set(i, j, Xprime.At(i, j)-wMean)
		}
	}

	// Weighted covariance: C = X'ᵀ * W * X' / (sum(W) - 1)
	// Apply sqrt(w) to each row so that X'ᵀ * X' becomes X'ᵀ * W * X'
	for i := range n {
		sw := math.Sqrt(weights[i])
		for j := range t {
			Xprime.Set(i, j, Xprime.At(i, j)*sw)
		}
	}
	var covRaw mat.Dense
	covRaw.Mul(Xprime.T(), &Xprime)
	covRaw.Scale(1.0/math.Max(sumW-1, 1), &covRaw)

	// Convert to SymDense for EigenSym
	covSym := mat.NewSymDense(t, nil)
	for i := range t {
		for j := i; j < t; j++ {
			covSym.SetSym(i, j, covRaw.At(i, j))
		}
	}

	// Eigendecompose
	var eig mat.EigenSym
	ok := eig.Factorize(covSym, true)
	if !ok {
		return nil, fmt.Errorf("factorize covariance matrix")
	}

	var eigVecs mat.Dense
	eig.VectorsTo(&eigVecs)

	// Top 2 eigenvectors (last 2 columns — gonum returns ascending order)
	v2Data := make([]float64, t*2)
	for i := range t {
		v2Data[i*2] = eigVecs.At(i, t-1)
		v2Data[i*2+1] = eigVecs.At(i, t-2)
	}
	V2 := mat.NewDense(t, 2, v2Data)

	// Orientation is decided in two layers: a data-driven sign convention
	// makes the layout independent of input order, then (when a previous
	// layout exists) Procrustes alignment overrides it with whatever
	// orientation users last saw.
	enforceLoadingSignConvention(V2)

	// Project: P = X' * V2
	var P mat.Dense
	P.Mul(&Xprime, V2)

	alignToPreviousLayout(&P, issues, previous)

	// Normalize to [0.05, 0.95] with outlier clipping so new extremes
	// are less likely to rescale the entire map.
	positions := make(map[string]Position, n)
	xs := make([]float64, n)
	ys := make([]float64, n)
	for i := range n {
		xs[i] = P.At(i, 0)
		ys[i] = P.At(i, 1)
	}

	normalizeRobust(xs, 0.05, 0.95)
	normalizeRobust(ys, 0.05, 0.95)

	for i, issue := range issues {
		positions[issue.ID] = Position{X: xs[i], Y: ys[i]}
	}

	return positions, nil
}

// enforceLoadingSignConvention fixes each principal axis to a property of
// the data instead of the input order: the tag with the largest absolute
// loading on the axis must load positively. Eigenvector signs are arbitrary,
// so without a convention the same corpus can come back mirrored.
func enforceLoadingSignConvention(loadings *mat.Dense) {
	rows, cols := loadings.Dims()
	for c := range cols {
		maxIdx, maxAbs := 0, 0.0
		for i := range rows {
			if a := math.Abs(loadings.At(i, c)); a > maxAbs {
				maxAbs = a
				maxIdx = i
			}
		}
		if loadings.At(maxIdx, c) < 0 {
			for i := range rows {
				loadings.Set(i, c, -loadings.At(i, c))
			}
		}
	}
}

// alignToPreviousLayout rotates/reflects the raw projected coordinates P
// (n×2, row i belongs to items[i]) so issues present in both layouts land
// as close as possible to where they were. The reference is the previous
// layout's normalized [0.05, 0.95] coordinates — the layout users actually
// saw — with no un-mapping: orthogonal Procrustes is invariant to the
// reference's translation (both point sets are centered) and its scale only
// multiplies the cross-covariance, so the optimal transform is unchanged,
// and the robust normalization afterwards re-establishes position and range.
// The transform is the full orthogonal solution Q = UVᵀ from the SVD of the
// 2×2 cross-covariance of the centered shared points — reflections are
// deliberately allowed so eigenvector sign flips and PC1/PC2 swaps are both
// corrected.
func alignToPreviousLayout(pts *mat.Dense, items []issues.Issue, previous map[string]Position) {
	if len(previous) == 0 {
		return
	}

	type pointPair struct {
		newX, newY, oldX, oldY float64
	}
	shared := make([]pointPair, 0, len(previous))
	for i, item := range items {
		prev, ok := previous[item.ID]
		if !ok {
			continue
		}
		shared = append(shared, pointPair{
			newX: pts.At(i, 0), newY: pts.At(i, 1),
			oldX: prev.X, oldY: prev.Y,
		})
	}
	if len(shared) < minAlignmentSharedIssues {
		return
	}

	k := float64(len(shared))
	var newCX, newCY, oldCX, oldCY float64
	for _, p := range shared {
		newCX += p.newX
		newCY += p.newY
		oldCX += p.oldX
		oldCY += p.oldY
	}
	newCX /= k
	newCY /= k
	oldCX /= k
	oldCY /= k

	// Cross-covariance M = AᵀB of centered shared points (A new, B old).
	var m00, m01, m10, m11 float64
	for _, p := range shared {
		ax, ay := p.newX-newCX, p.newY-newCY
		bx, by := p.oldX-oldCX, p.oldY-oldCY
		m00 += ax * bx
		m01 += ax * by
		m10 += ay * bx
		m11 += ay * by
	}
	crossCov := mat.NewDense(2, 2, []float64{m00, m01, m10, m11})

	var svd mat.SVD
	if !svd.Factorize(crossCov, mat.SVDFull) {
		return
	}
	// Coincident or collinear-degenerate anchors carry no orientation
	// signal; keep the sign-convention orientation rather than applying
	// an arbitrary transform.
	if values := svd.Values(nil); values[0] <= 1e-12 {
		return
	}

	var u, v mat.Dense
	svd.UTo(&u)
	svd.VTo(&v)
	var q mat.Dense
	q.Mul(&u, v.T())

	var aligned mat.Dense
	aligned.Mul(pts, &q)
	pts.Copy(&aligned)
}

func normalizeRobust(vals []float64, lo, hi float64) {
	if len(vals) == 0 {
		return
	}

	sorted := append([]float64(nil), vals...)
	slices.Sort(sorted)

	q1 := percentile(sorted, 0.25)
	q3 := percentile(sorted, 0.75)
	iqr := q3 - q1
	if iqr <= 0 {
		normalizeMinMax(vals, lo, hi)
		return
	}

	minV, maxV := sorted[0], sorted[len(sorted)-1]
	lower := math.Max(minV, q1-(1.5*iqr))
	upper := math.Min(maxV, q3+(1.5*iqr))
	if upper <= lower {
		normalizeMinMax(vals, lo, hi)
		return
	}

	for i, v := range vals {
		clamped := min(max(v, lower), upper)
		vals[i] = lo + ((clamped - lower) / (upper - lower) * (hi - lo))
	}
}

func normalizeMinMax(vals []float64, lo, hi float64) {
	minV, maxV := vals[0], vals[0]
	for _, v := range vals[1:] {
		minV = math.Min(minV, v)
		maxV = math.Max(maxV, v)
	}
	span := maxV - minV
	if span == 0 {
		mid := (lo + hi) / 2
		for i := range vals {
			vals[i] = mid
		}
		return
	}
	for i, v := range vals {
		vals[i] = lo + (v-minV)/span*(hi-lo)
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}

	index := p * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	if lower == upper {
		return sorted[lower]
	}

	weight := index - float64(lower)
	return sorted[lower] + (sorted[upper]-sorted[lower])*weight
}

// BuildTagCovariance computes a T×T matrix where entry (i,j) is the cosine
// similarity between the embeddings of tag i and tag j. If embeddings are
// missing, falls back to the identity matrix (tags treated as independent).
func BuildTagCovariance(tags []string, tagEmbeddings map[string][]float64) *mat.Dense {
	return buildTagCovariance(tags, tagEmbeddings)
}

func buildTagCovariance(tags []string, tagEmbeddings map[string][]float64) *mat.Dense {
	t := len(tags)
	data := make([]float64, t*t)

	hasEmbeddings := len(tagEmbeddings) > 0

	// Pre-extract tag embeddings into indexed slice to avoid O(t²) hash lookups.
	var indexed [][]float64
	if hasEmbeddings {
		indexed = make([][]float64, t)
		for i, tag := range tags {
			indexed[i] = tagEmbeddings[tag]
		}
	}

	for i := range t {
		for j := i; j < t; j++ {
			value := 0.0
			if hasEmbeddings {
				if indexed[i] != nil && indexed[j] != nil {
					value = vectors.UnitCosineSimilarity(indexed[i], indexed[j])
				} else if i == j {
					value = 1
				}
			} else if i == j {
				value = 1
			}
			data[i*t+j] = value
			data[j*t+i] = value
		}
	}

	// Correlation shrinkage heuristic: Σ_shrunk = α·Σ + (1-α)·I.
	// Pulls the similarity matrix toward the identity, regularizing PCA
	// and factor decomposition as the tag catalog grows.
	// Diagonal is already 1.0 (self-similarity), so only off-diagonal scales.
	if hasEmbeddings && t > 1 {
		alpha := correlationShrinkageAlpha(data, t)
		for i := range t {
			for j := range t {
				if i != j {
					data[i*t+j] *= alpha
				}
			}
		}
	}

	return mat.NewDense(t, t, data)
}

// correlationShrinkageAlpha computes a shrinkage intensity for a T×T
// correlation-like matrix (diagonal = 1) stored in row-major order.
// Returns α ∈ [0.1, 1.0], the weight on the original matrix in:
//
//	Σ_shrunk = α·Σ + (1-α)·I
//
// This is a simple regularization heuristic for the semantic similarity matrix:
// the mean squared off-diagonal entry measures how far the matrix is from
// identity. Higher off-diagonal energy leads to stronger shrinkage.
func correlationShrinkageAlpha(data []float64, t int) float64 {
	if t <= 1 {
		return 1.0
	}

	var offDiagSumSq float64
	for i := range t {
		for j := range t {
			if i != j {
				v := data[i*t+j]
				offDiagSumSq += v * v
			}
		}
	}

	// Mean squared off-diagonal entry ∈ [0, 1].
	meanSqOffDiag := offDiagSumSq / float64(t*(t-1))

	// α = 1 when matrix is identity (no off-diagonal energy).
	// α → 0.1 as mean squared off-diagonal → 1.
	return clamp(1.0-meanSqOffDiag, 0.1, 1.0)
}

// issueProjectionWeights computes a per-issue weight for the PCA covariance.
// Weight = contentConfidence * maturity. Both are 0..1, so the product is 0..1.
// A floor of 0.1 ensures no issue is completely ignored.
func issueProjectionWeights(items []issues.Issue) []float64 {
	weights := make([]float64, len(items))
	for i, item := range items {
		cc := issueanalytics.ComputeContentConfidence(item.Raw)
		maturity := scoring.DefaultMaturity
		if item.LifecycleMetrics != nil && item.LifecycleMetrics.Maturity != nil {
			maturity = *item.LifecycleMetrics.Maturity
		}
		w := cc * maturity
		if w < scoring.ProjectionWeightFloor {
			w = scoring.ProjectionWeightFloor
		}
		weights[i] = w
	}
	return weights
}
