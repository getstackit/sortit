package issuethemes

import (
	"math"
	"testing"
)

func TestBuildDerivesDeterministicThemes(t *testing.T) {
	tagNames := []string{"auth", "login", "billing", "invoice"}
	loadings := []IssueLoading{
		{IssueID: "auth-1", Values: []float64{1.0, 0.8, 0, 0}},
		{IssueID: "auth-2", Values: []float64{0.9, 0.7, 0, 0}},
		{IssueID: "billing-1", Values: []float64{0, 0, 1.0, 0.8}},
		{IssueID: "billing-2", Values: []float64{0, 0, 0.9, 0.7}},
	}
	embeddings := map[string][]float64{
		"auth":    {1, 0, 0},
		"login":   {0.9, 0.1, 0},
		"billing": {0, 1, 0},
		"invoice": {0.1, 0.9, 0},
	}

	first := Build(loadings, tagNames, embeddings, Options{ThemeCount: 2, Iterations: 60, TopTags: 2})
	second := Build(loadings, tagNames, embeddings, Options{ThemeCount: 2, Iterations: 60, TopTags: 2})

	if len(first.Themes) != 2 || len(first.IssueThemes) != 4 {
		t.Fatalf("expected two themes and four issue rows, got %+v", first)
	}
	if !sameResult(first, second) {
		t.Fatalf("expected deterministic result\nfirst:  %+v\nsecond: %+v", first, second)
	}
	for _, theme := range first.Themes {
		if len(theme.Tags) != 2 {
			t.Fatalf("expected top two tags for theme %+v", theme)
		}
		if theme.Tags[0].Loading < theme.Tags[1].Loading {
			t.Fatalf("expected tags sorted by loading, got %+v", theme.Tags)
		}
		if got := norm(theme.Centroid); math.Abs(got-1) > 1e-9 {
			t.Fatalf("expected unit centroid, got norm %v for %+v", got, theme)
		}
	}

	firstTags := []string{first.Themes[0].Tags[0].Tag, first.Themes[1].Tags[0].Tag}
	if !contains(firstTags, "auth") || !contains(firstTags, "billing") {
		t.Fatalf("expected auth and billing themes, got %+v", first.Themes)
	}
}

func TestBuildUsesPositivePartsOnly(t *testing.T) {
	result := Build([]IssueLoading{
		{IssueID: "negated", Values: []float64{-0.8, 0.6}},
	}, []string{"backend", "signed-relevance"}, nil, Options{ThemeCount: 1, Iterations: 10})

	if len(result.Themes) != 1 {
		t.Fatalf("expected one positive theme, got %+v", result)
	}
	if result.Themes[0].Tags[0].Tag != "signed-relevance" {
		t.Fatalf("expected negative loading to be ignored, got %+v", result.Themes[0].Tags)
	}
}

func TestBuildHandlesEmptyAndDegenerateInputs(t *testing.T) {
	if got := Build(nil, []string{"auth"}, nil, Options{}); len(got.Themes) != 0 {
		t.Fatalf("expected empty result for no rows, got %+v", got)
	}
	if got := Build([]IssueLoading{{IssueID: "a", Values: []float64{-1}}}, []string{"auth"}, nil, Options{}); len(got.Themes) != 0 {
		t.Fatalf("expected empty result for no positive mass, got %+v", got)
	}
	if got := Build([]IssueLoading{{IssueID: "a", Values: []float64{1}}}, nil, nil, Options{}); len(got.Themes) != 0 {
		t.Fatalf("expected empty result for no tags, got %+v", got)
	}
}

func TestBuildCapsThemeCountToMatrixRankShape(t *testing.T) {
	result := Build([]IssueLoading{
		{IssueID: "one", Values: []float64{1, 0}},
	}, []string{"auth", "billing"}, nil, Options{ThemeCount: 8})

	if len(result.Themes) != 1 || len(result.IssueThemes[0].Weights) != 1 {
		t.Fatalf("expected theme count capped to one row, got %+v", result)
	}
}

// denseCorpus builds a deterministic dense non-negative loading matrix (a small
// LCG, no imports) with no clean low-rank structure — MU improves it slowly, so
// it does not converge within a handful of iterations. Used to exercise the
// cap-hit and monotonicity paths where a cleanly separable corpus would converge
// almost immediately.
func denseCorpus(rows, cols int) ([]IssueLoading, []string) {
	loadings := make([]IssueLoading, rows)
	seed := uint64(0x9e3779b97f4a7c15)
	next := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return float64(seed>>11) / float64(1<<53)
	}
	for i := range rows {
		vals := make([]float64, cols)
		for j := range cols {
			vals[j] = 0.1 + next() // strictly positive, so every row participates
		}
		loadings[i] = IssueLoading{IssueID: intID(i), Values: vals}
	}
	tagNames := make([]string, cols)
	for j := range cols {
		tagNames[j] = "tag-" + intID(j)
	}
	return loadings, tagNames
}

func intID(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

// TestBuildEarlyStopConvergesBeforeCap: a cleanly separable corpus reaches the
// convergence floor well before the 200-iteration backstop, and reports the
// iterations it used plus a small reconstruction error.
func TestBuildEarlyStopConvergesBeforeCap(t *testing.T) {
	tagNames := []string{"auth", "login", "billing", "invoice"}
	loadings := []IssueLoading{
		{IssueID: "auth-1", Values: []float64{1.0, 0.8, 0, 0}},
		{IssueID: "auth-2", Values: []float64{0.9, 0.7, 0, 0}},
		{IssueID: "billing-1", Values: []float64{0, 0, 1.0, 0.8}},
		{IssueID: "billing-2", Values: []float64{0, 0, 0.9, 0.7}},
	}

	res := Build(loadings, tagNames, nil, Options{ThemeCount: 2})
	if res.Iterations <= 0 {
		t.Fatalf("expected a positive iteration count, got %d", res.Iterations)
	}
	if res.Iterations >= defaultIterations {
		t.Fatalf("separable corpus should converge before the %d cap, used %d", defaultIterations, res.Iterations)
	}
	if res.ReconstructionError < 0 || res.ReconstructionError > 1 {
		t.Fatalf("reconstruction error out of [0,1]: %v", res.ReconstructionError)
	}
}

// TestBuildEarlyStopHitsCap: a dense, slow-converging corpus with a small cap
// runs every iteration — the cap, not the convergence floor, terminates it.
func TestBuildEarlyStopHitsCap(t *testing.T) {
	loadings, tagNames := denseCorpus(24, 14)

	const cap = 5
	res := Build(loadings, tagNames, nil, Options{Iterations: cap})
	if res.Iterations != cap {
		t.Fatalf("dense corpus at cap=%d should run every iteration, used %d", cap, res.Iterations)
	}
}

// TestBuildEarlyStopDeterministic: two runs of the same input agree on every
// output, including the stopping iteration and the reconstruction error.
func TestBuildEarlyStopDeterministic(t *testing.T) {
	loadings, tagNames := denseCorpus(20, 10)

	a := Build(loadings, tagNames, nil, Options{})
	b := Build(loadings, tagNames, nil, Options{})
	if a.Iterations != b.Iterations {
		t.Fatalf("iterations differ across runs: %d vs %d", a.Iterations, b.Iterations)
	}
	if math.Abs(a.ReconstructionError-b.ReconstructionError) > 1e-12 {
		t.Fatalf("reconstruction error differs across runs: %v vs %v", a.ReconstructionError, b.ReconstructionError)
	}
	if !sameResult(a, b) {
		t.Fatalf("factorization not deterministic across runs")
	}
}

// TestMultiplicativeUpdatesMonotone: the Frobenius objective is non-increasing
// across every MU iteration (Lee–Seung guarantees it; asserting it catches a
// sign or transposition error in the update rules).
func TestMultiplicativeUpdatesMonotone(t *testing.T) {
	loadings, tagNames := denseCorpus(24, 14)
	v := positiveMatrix(loadings, len(tagNames))
	w, h := initializeNNDSVD(v, 8)

	prev := math.Inf(1)
	for iter := 0; iter < 40; iter++ {
		updateH(v, w, h)
		updateW(v, w, h)
		obj := frobeniusResidual(v, w, h)
		// Allow a hair of floating-point slack; the guarantee is exact in reals.
		if obj > prev+1e-9 {
			t.Fatalf("objective increased at iteration %d: %v -> %v", iter, prev, obj)
		}
		prev = obj
	}
}

func sameResult(a, b Result) bool {
	if len(a.Themes) != len(b.Themes) || len(a.IssueThemes) != len(b.IssueThemes) {
		return false
	}
	for i := range a.Themes {
		if math.Abs(a.Themes[i].Weight-b.Themes[i].Weight) > 1e-12 {
			return false
		}
		if len(a.Themes[i].Tags) != len(b.Themes[i].Tags) {
			return false
		}
		for j := range a.Themes[i].Tags {
			if a.Themes[i].Tags[j].Tag != b.Themes[i].Tags[j].Tag ||
				math.Abs(a.Themes[i].Tags[j].Loading-b.Themes[i].Tags[j].Loading) > 1e-12 {
				return false
			}
		}
	}
	for i := range a.IssueThemes {
		if a.IssueThemes[i].IssueID != b.IssueThemes[i].IssueID ||
			len(a.IssueThemes[i].Weights) != len(b.IssueThemes[i].Weights) {
			return false
		}
		for j := range a.IssueThemes[i].Weights {
			if math.Abs(a.IssueThemes[i].Weights[j]-b.IssueThemes[i].Weights[j]) > 1e-12 {
				return false
			}
		}
	}
	return true
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
