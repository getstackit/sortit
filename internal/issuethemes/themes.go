// Package issuethemes derives planning themes from positive ridge loadings.
package issuethemes

import (
	"cmp"
	"math"
	"slices"

	"gonum.org/v1/gonum/mat"
)

const (
	defaultThemeCount = 8
	// defaultIterations is the multiplicative-update cap. With early stop
	// (convergenceEpsilon) in place it is a backstop, not the terminator: a
	// well-conditioned corpus converges and stops well before it, and only a
	// pathological, slow-converging factorization runs the whole budget.
	defaultIterations = 200
	// convergenceEpsilon is the relative-improvement floor for the Frobenius
	// objective ‖V−WH‖_F: once one full MU iteration (H then W) improves the
	// objective by less than this fraction of the previous objective, the loop
	// stops. MU decreases the objective monotonically, so the check is a
	// one-sided "we've stopped making progress" test. It is deterministic — the
	// same input reaches the same stopping iteration — so bit-for-bit
	// reproducibility is preserved.
	convergenceEpsilon = 1e-4
	epsilon            = 1e-12
)

// Options controls theme extraction.
type Options struct {
	ThemeCount int
	Iterations int
	TopTags    int
}

// IssueLoading is one issue's tag-space ridge loading vector f.
type IssueLoading struct {
	IssueID string
	Values  []float64
}

// Result is the non-negative factorization F+ ~= W*H.
type Result struct {
	IssueThemes []IssueThemes
	Themes      []Theme
	// Iterations is the number of multiplicative-update iterations actually run
	// before the objective converged (relative improvement < convergenceEpsilon)
	// or the cap was hit. Zero for a degenerate/empty input that never entered
	// the MU loop. Surfaced as convergence telemetry (WP-206).
	Iterations int
	// ReconstructionError is the final relative Frobenius reconstruction error
	// ‖V−WH‖_F / ‖V‖_F over the positive-part input matrix V — a normalized,
	// K-comparable measure of how much of the corpus the factorization explains
	// (0 = perfect, 1 = no better than the zero matrix). Zero for a degenerate
	// input with no positive mass.
	ReconstructionError float64
}

// IssueThemes is one issue's row of W: its theme membership weights.
type IssueThemes struct {
	IssueID string
	Weights []float64
}

// Theme is one row of H plus derived interpretation helpers.
type Theme struct {
	Index    int
	Weight   float64
	Tags     []ThemeTag
	Centroid []float64
}

// ThemeTag is one tag loading within a theme.
type ThemeTag struct {
	Tag     string
	Loading float64
}

// Build runs deterministic NMF over max(0, f_i) ridge loadings.
func Build(loadings []IssueLoading, tagNames []string, tagEmbeddings map[string][]float64, opts Options) Result {
	rows := positiveMatrix(loadings, len(tagNames))
	if len(rows) == 0 || len(tagNames) == 0 {
		return Result{}
	}

	k := opts.ThemeCount
	if k <= 0 {
		k = defaultThemeCount
	}
	k = min(k, len(rows), len(tagNames))
	if k <= 0 {
		return Result{}
	}
	iterations := opts.Iterations
	if iterations <= 0 {
		iterations = defaultIterations
	}
	topTags := opts.TopTags
	if topTags <= 0 {
		topTags = 5
	}

	w, h := initializeNNDSVD(rows, k)
	used := runMultiplicativeUpdates(rows, w, h, iterations)
	normalizeComponents(w, h)

	order := themeOrder(w, h, tagNames)
	result := buildResult(loadings, tagNames, tagEmbeddings, w, h, order, topTags)
	result.Iterations = used
	// Normalization pushes mass between W and H but leaves the product W·H (and
	// therefore the residual) exactly invariant, so this measures the same
	// factorization the MU loop converged to.
	result.ReconstructionError = reconstructionError(rows, w, h)
	return result
}

// runMultiplicativeUpdates runs Lee–Seung MU (H then W) up to cap iterations,
// stopping early once the relative improvement in the Frobenius objective
// ‖V−WH‖_F drops below convergenceEpsilon. It returns the number of iterations
// actually run. Deterministic: the same V/W/H reach the same stopping point.
func runMultiplicativeUpdates(v, w, h [][]float64, cap int) int {
	prev := math.Inf(1)
	used := 0
	for range cap {
		updateH(v, w, h)
		updateW(v, w, h)
		used++
		obj := frobeniusResidual(v, w, h)
		if !math.IsInf(prev, 1) {
			// MU is monotone non-increasing, so obj <= prev; a zero previous
			// objective is already a perfect fit and further steps cannot help.
			if prev <= 0 || (prev-obj)/prev < convergenceEpsilon {
				break
			}
		}
		prev = obj
	}
	return used
}

// frobeniusResidual returns ‖V−WH‖_F, the unnormalized reconstruction error the
// MU loop drives toward zero.
func frobeniusResidual(v, w, h [][]float64) float64 {
	if len(v) == 0 || len(w) == 0 || len(h) == 0 {
		return 0
	}
	n, m, k := len(v), len(v[0]), len(w[0])
	sum := 0.0
	for i := range n {
		for j := range m {
			approx := 0.0
			for c := range k {
				approx += w[i][c] * h[c][j]
			}
			d := v[i][j] - approx
			sum += d * d
		}
	}
	return math.Sqrt(sum)
}

// reconstructionError returns the relative Frobenius error ‖V−WH‖_F / ‖V‖_F,
// or 0 when V has no mass.
func reconstructionError(v, w, h [][]float64) float64 {
	vNorm := 0.0
	for _, row := range v {
		for _, x := range row {
			vNorm += x * x
		}
	}
	vNorm = math.Sqrt(vNorm)
	if vNorm <= 0 {
		return 0
	}
	return frobeniusResidual(v, w, h) / vNorm
}

func positiveMatrix(loadings []IssueLoading, width int) [][]float64 {
	if width == 0 {
		return nil
	}
	rows := make([][]float64, 0, len(loadings))
	for _, loading := range loadings {
		if loading.IssueID == "" || len(loading.Values) != width {
			continue
		}
		row := make([]float64, width)
		any := false
		for i, v := range loading.Values {
			if v > 0 {
				row[i] = v
				any = true
			}
		}
		if any {
			rows = append(rows, row)
		}
	}
	return rows
}

func initializeNNDSVD(v [][]float64, k int) ([][]float64, [][]float64) {
	n, m := len(v), len(v[0])
	data := make([]float64, 0, n*m)
	mean := 0.0
	for _, row := range v {
		for _, x := range row {
			data = append(data, x)
			mean += x
		}
	}
	mean /= float64(n * m)
	if mean <= 0 {
		mean = epsilon
	}

	var svd mat.SVD
	if !svd.Factorize(mat.NewDense(n, m, data), mat.SVDThin) {
		return seededFallback(v, k, mean)
	}
	values := svd.Values(nil)
	var u, vt mat.Dense
	svd.UTo(&u)
	svd.VTo(&vt)

	w := makeMatrix(n, k)
	h := makeMatrix(k, m)
	for comp := range k {
		if comp >= len(values) || values[comp] <= 0 {
			fillFallbackComponent(v, w, h, comp, mean)
			continue
		}
		if comp == 0 {
			scale := math.Sqrt(values[comp])
			for i := range n {
				w[i][comp] = max(math.Abs(u.At(i, comp))*scale, epsilon)
			}
			for j := range m {
				h[comp][j] = max(math.Abs(vt.At(j, comp))*scale, epsilon)
			}
			continue
		}

		uPos, uNeg := splitSignedColumn(&u, comp, n)
		vPos, vNeg := splitSignedColumn(&vt, comp, m)
		uPosNorm, uNegNorm := norm(uPos), norm(uNeg)
		vPosNorm, vNegNorm := norm(vPos), norm(vNeg)
		posMass := uPosNorm * vPosNorm
		negMass := uNegNorm * vNegNorm

		left, right := uPos, vPos
		leftNorm, rightNorm, mass := uPosNorm, vPosNorm, posMass
		if negMass > posMass {
			left, right = uNeg, vNeg
			leftNorm, rightNorm, mass = uNegNorm, vNegNorm, negMass
		}
		if leftNorm == 0 || rightNorm == 0 || mass == 0 {
			fillFallbackComponent(v, w, h, comp, mean)
			continue
		}
		scale := math.Sqrt(values[comp] * mass)
		for i := range n {
			w[i][comp] = max(scale*left[i]/leftNorm, epsilon)
		}
		for j := range m {
			h[comp][j] = max(scale*right[j]/rightNorm, epsilon)
		}
	}
	return w, h
}

func splitSignedColumn(values mat.Matrix, col, length int) ([]float64, []float64) {
	pos := make([]float64, length)
	neg := make([]float64, length)
	for i := range length {
		v := values.At(i, col)
		if v >= 0 {
			pos[i] = v
		} else {
			neg[i] = -v
		}
	}
	return pos, neg
}

func seededFallback(v [][]float64, k int, mean float64) ([][]float64, [][]float64) {
	w := makeMatrix(len(v), k)
	h := makeMatrix(k, len(v[0]))
	for comp := range k {
		fillFallbackComponent(v, w, h, comp, mean)
	}
	return w, h
}

func fillFallbackComponent(v [][]float64, w, h [][]float64, comp int, mean float64) {
	for i := range w {
		w[i][comp] = mean * (1 + 0.01*float64((i+comp)%3))
	}
	for j := range h[comp] {
		h[comp][j] = max(v[j%len(v)][j]*0.1, mean*0.1)
	}
}

func updateH(v, w, h [][]float64) {
	n, k, m := len(v), len(w[0]), len(v[0])
	next := makeMatrix(k, m)
	for comp := range k {
		for tag := range m {
			num := 0.0
			den := 0.0
			for issue := range n {
				num += w[issue][comp] * v[issue][tag]
				approx := 0.0
				for other := range k {
					approx += w[issue][other] * h[other][tag]
				}
				den += w[issue][comp] * approx
			}
			next[comp][tag] = h[comp][tag] * num / max(den, epsilon)
		}
	}
	copyMatrix(h, next)
}

func updateW(v, w, h [][]float64) {
	n, k, m := len(v), len(w[0]), len(v[0])
	next := makeMatrix(n, k)
	for issue := range n {
		for comp := range k {
			num := 0.0
			den := 0.0
			for tag := range m {
				num += v[issue][tag] * h[comp][tag]
				approx := 0.0
				for other := range k {
					approx += w[issue][other] * h[other][tag]
				}
				den += approx * h[comp][tag]
			}
			next[issue][comp] = w[issue][comp] * num / max(den, epsilon)
		}
	}
	copyMatrix(w, next)
}

func normalizeComponents(w, h [][]float64) {
	if len(w) == 0 || len(h) == 0 {
		return
	}
	for comp := range h {
		sum := 0.0
		for _, v := range h[comp] {
			sum += v
		}
		if sum <= 0 {
			continue
		}
		for tag := range h[comp] {
			h[comp][tag] /= sum
		}
		for issue := range w {
			w[issue][comp] *= sum
		}
	}
}

func themeOrder(w, h [][]float64, tagNames []string) []int {
	order := make([]int, len(h))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		aWeight, bWeight := componentWeight(w, a), componentWeight(w, b)
		if aWeight != bWeight {
			return cmp.Compare(bWeight, aWeight)
		}
		return cmp.Compare(topTagName(h[a], tagNames), topTagName(h[b], tagNames))
	})
	return order
}

func buildResult(loadings []IssueLoading, tagNames []string, tagEmbeddings map[string][]float64, w, h [][]float64, order []int, topTags int) Result {
	issueRows := make([]IssueThemes, 0, len(loadings))
	rowIndex := 0
	for _, loading := range loadings {
		if loading.IssueID == "" || len(loading.Values) != len(tagNames) || !hasPositive(loading.Values) {
			continue
		}
		weights := make([]float64, len(order))
		for out, comp := range order {
			weights[out] = w[rowIndex][comp]
		}
		issueRows = append(issueRows, IssueThemes{IssueID: loading.IssueID, Weights: weights})
		rowIndex++
	}

	themes := make([]Theme, 0, len(order))
	for out, comp := range order {
		themes = append(themes, Theme{
			Index:    out,
			Weight:   componentWeight(w, comp),
			Tags:     topThemeTags(h[comp], tagNames, topTags),
			Centroid: themeCentroid(h[comp], tagNames, tagEmbeddings),
		})
	}
	return Result{IssueThemes: issueRows, Themes: themes}
}

func topThemeTags(row []float64, tagNames []string, limit int) []ThemeTag {
	tags := make([]ThemeTag, 0, len(row))
	for i, loading := range row {
		if loading <= 0 || i >= len(tagNames) || tagNames[i] == "" {
			continue
		}
		tags = append(tags, ThemeTag{Tag: tagNames[i], Loading: loading})
	}
	slices.SortStableFunc(tags, func(a, b ThemeTag) int {
		if a.Loading != b.Loading {
			return cmp.Compare(b.Loading, a.Loading)
		}
		return cmp.Compare(a.Tag, b.Tag)
	})
	if len(tags) > limit {
		tags = tags[:limit]
	}
	return tags
}

func themeCentroid(row []float64, tagNames []string, tagEmbeddings map[string][]float64) []float64 {
	dim := 0
	for _, tag := range tagNames {
		if emb := tagEmbeddings[tag]; len(emb) > 0 {
			dim = len(emb)
			break
		}
	}
	if dim == 0 {
		return nil
	}
	out := make([]float64, dim)
	for i, loading := range row {
		if loading <= 0 || i >= len(tagNames) {
			continue
		}
		emb := tagEmbeddings[tagNames[i]]
		if len(emb) != dim {
			continue
		}
		for j, v := range emb {
			out[j] += loading * v
		}
	}
	if n := norm(out); n > 0 {
		for i := range out {
			out[i] /= n
		}
		return out
	}
	return nil
}

func componentWeight(w [][]float64, comp int) float64 {
	sum := 0.0
	for _, row := range w {
		sum += row[comp]
	}
	return sum
}

func topTagName(row []float64, tagNames []string) string {
	best := ""
	bestValue := -1.0
	for i, v := range row {
		if i >= len(tagNames) {
			continue
		}
		if v > bestValue || (v == bestValue && tagNames[i] < best) {
			best = tagNames[i]
			bestValue = v
		}
	}
	return best
}

func hasPositive(values []float64) bool {
	for _, v := range values {
		if v > 0 {
			return true
		}
	}
	return false
}

func makeMatrix(rows, cols int) [][]float64 {
	out := make([][]float64, rows)
	for i := range out {
		out[i] = make([]float64, cols)
	}
	return out
}

func copyMatrix(dst, src [][]float64) {
	for i := range dst {
		copy(dst[i], src[i])
	}
}

func norm(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v * v
	}
	return math.Sqrt(sum)
}
