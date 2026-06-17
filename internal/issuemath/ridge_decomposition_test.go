package issuemath

import (
	"math"
	"testing"

	"sortit/internal/issues"
	"sortit/internal/scoring"
)

func ridgeTestItems(embeddings map[string][]float64, tagsByIssue map[string][]issues.TagRelevance) []issues.Issue {
	items := make([]issues.Issue, 0, len(embeddings))
	for id := range embeddings {
		items = append(items, issues.Issue{ID: id, TagScores: tagsByIssue[id]})
	}
	return items
}

// TestComputeRidgeDecomposition_ReconstructionAndR2 checks the core
// invariants: the residual is e − Tᵀf, the stored R² equals
// 1 − ‖residual‖²/‖e‖², and a well-aligned embedding reconstructs better
// (higher R²) than a noisy one.
func TestComputeRidgeDecomposition_ReconstructionAndR2(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
		"beta":  unitVec([]float64{0, 1, 0, 0}),
		"gamma": unitVec([]float64{0, 0, 1, 0}),
	}
	tagNames := []string{"alpha", "beta", "gamma"}

	embeddings := map[string][]float64{
		"clean-A": unitVec([]float64{0.95, 0.30, 0, 0}),
		"clean-B": unitVec([]float64{0.90, 0.40, 0, 0}),
		"clean-C": unitVec([]float64{0.30, 0.95, 0, 0}),
		"noisy-A": unitVec([]float64{0.3, 0.2, 0.1, 0.93}), // mostly off-taxonomy axis
		"noisy-B": unitVec([]float64{0.2, 0.1, 0.2, 0.95}),
	}
	tagsByIssue := map[string][]issues.TagRelevance{
		"clean-A": {{Tag: "alpha", Relevance: 0.9}, {Tag: "beta", Relevance: 0.3}},
		"clean-B": {{Tag: "alpha", Relevance: 0.85}, {Tag: "beta", Relevance: 0.4}},
		"clean-C": {{Tag: "beta", Relevance: 0.9}, {Tag: "alpha", Relevance: 0.3}},
		"noisy-A": {{Tag: "alpha", Relevance: 0.4}},
		"noisy-B": {{Tag: "alpha", Relevance: 0.3}},
	}

	d := ComputeRidgeDecomposition(ridgeTestItems(embeddings, tagsByIssue), tagNames, embeddings, tagEmb,
		scoring.RidgeAnchorLambdaScored, scoring.RidgeAnchorLambdaUnscored)
	if !d.Decomposed() {
		t.Fatal("expected ridge decomposition to run")
	}

	clean, ok := d.VectorsFor("clean-A")
	if !ok {
		t.Fatal("clean-A missing from decomposition")
	}
	// R² is consistent with the stored residual norm and a unit embedding.
	wantR2 := 1 - clean.ResidualNorm*clean.ResidualNorm
	if math.Abs(clean.R2-wantR2) > 1e-9 {
		t.Errorf("R² = %f, want 1 − residualNorm² = %f", clean.R2, wantR2)
	}
	if clean.R2 <= 0 || clean.R2 > 1 {
		t.Errorf("a well-aligned embedding should have R² in (0,1], got %f", clean.R2)
	}

	noisy, _ := d.VectorsFor("noisy-A")
	if noisy.R2 >= clean.R2 {
		t.Errorf("off-taxonomy embedding R² %f should be below well-aligned R² %f", noisy.R2, clean.R2)
	}

	// Aggregate R² is the corpus variance-explained and drives the weight.
	if d.AggregateR2 <= 0 || d.AggregateR2 >= 1 {
		t.Errorf("aggregate R² should be a non-trivial fraction, got %f", d.AggregateR2)
	}
	if d.FactorWeight < scoring.MinFactorWeight || d.FactorWeight > scoring.MaxFactorWeight {
		t.Errorf("factor weight %f out of clamp range", d.FactorWeight)
	}
}

// TestComputeRidgeDecomposition_CollinearTagsStayBounded is the conditioning
// guard flagged in the review: near-duplicate tags make T Tᵀ ill-conditioned,
// and the weak unscored penalty could let paired loadings blow up in opposite
// directions. The diagonal Λ (every tag penalized) must keep the solve finite
// and the reconstruction sane.
func TestComputeRidgeDecomposition_CollinearTagsStayBounded(t *testing.T) {
	// auth and login point in almost the same direction.
	tagEmb := map[string][]float64{
		"auth":  unitVec([]float64{1, 0, 0, 0}),
		"login": unitVec([]float64{0.999, 0.0447, 0, 0}),
		"other": unitVec([]float64{0, 0, 1, 0}),
	}
	tagNames := []string{"auth", "login", "other"}

	embeddings := map[string][]float64{}
	tagsByIssue := map[string][]issues.TagRelevance{}
	for i, id := range []string{"i1", "i2", "i3", "i4", "i5"} {
		embeddings[id] = unitVec([]float64{0.9, 0.1, 0.1 * float64(i), 0})
		// Only auth is scored; login is an unscored near-duplicate that could
		// destabilize the solve if the penalty were absent.
		tagsByIssue[id] = []issues.TagRelevance{{Tag: "auth", Relevance: 0.8}}
	}

	d := ComputeRidgeDecomposition(ridgeTestItems(embeddings, tagsByIssue), tagNames, embeddings, tagEmb,
		scoring.RidgeAnchorLambdaScored, scoring.RidgeAnchorLambdaUnscored)
	if !d.Decomposed() {
		t.Fatal("expected decomposition with collinear tags")
	}
	for _, id := range []string{"i1", "i2", "i3", "i4", "i5"} {
		v, ok := d.VectorsFor(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		for k, x := range v.Loading {
			if math.IsNaN(x) || math.IsInf(x, 0) {
				t.Fatalf("%s loading[%d] is non-finite: %v", id, k, x)
			}
		}
		if v.ReconNorm > 10 {
			t.Errorf("%s reconstruction norm %f implausibly large for collinear tags", id, v.ReconNorm)
		}
	}
}

// TestRidgeBlend_MarginalizesMissingEvidence mirrors the rank-1 degenerate
// rule for both similarity modes: a zero factor side puts full weight on the
// residual, a zero residual side puts full weight on the factor.
func TestRidgeBlend_MarginalizesMissingEvidence(t *testing.T) {
	d := RidgeDecomposition{used: true, FactorWeight: 0.6, ResidualWeight: 0.4}

	loadDir := unitVec([]float64{1, 0, 0})
	reconDir := unitVec([]float64{1, 0, 0, 0})
	residDir := unitVec([]float64{0, 1, 0, 0})

	full := RidgeVectors{Loading: loadDir, Reconstruction: reconDir, Residual: residDir}
	noFactor := RidgeVectors{Loading: nil, Reconstruction: nil, Residual: residDir}

	for _, mode := range []RidgeSimilarityMode{RidgeTagSpace, RidgeReconSpace} {
		_, _, blended := RidgeBlend(d, full, noFactor, mode)
		if math.Abs(blended-1) > 1e-9 {
			t.Errorf("mode %d: zero-factor pair should marginalize to residualSim=1, got %f", mode, blended)
		}
	}

	noResidual := RidgeVectors{Loading: loadDir, Reconstruction: reconDir, Residual: nil}
	if _, _, blended := RidgeBlend(d, full, noResidual, RidgeTagSpace); math.Abs(blended-1) > 1e-9 {
		t.Errorf("zero-residual pair should marginalize to factorSim=1, got %f", blended)
	}
}

// TestDecomposeRidgeEmbedding_MatchesCorpusSolve checks that the query-side
// decomposition produces the same vectors as the corpus path for an
// identical embedding and tag set — the property the search wiring relies on.
func TestDecomposeRidgeEmbedding_MatchesCorpusSolve(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
		"beta":  unitVec([]float64{0, 1, 0, 0}),
	}
	tagNames := []string{"alpha", "beta"}
	embeddings := map[string][]float64{}
	tagsByIssue := map[string][]issues.TagRelevance{}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		embeddings[id] = unitVec([]float64{0.8, 0.4, 0.2, 0})
		tagsByIssue[id] = []issues.TagRelevance{{Tag: "alpha", Relevance: 0.7}, {Tag: "beta", Relevance: 0.3}}
	}

	d := ComputeRidgeDecomposition(ridgeTestItems(embeddings, tagsByIssue), tagNames, embeddings, tagEmb,
		scoring.RidgeAnchorLambdaScored, scoring.RidgeAnchorLambdaUnscored)

	corpus, ok := d.VectorsFor("a")
	if !ok {
		t.Fatal("issue a missing")
	}
	query := DecomposeRidgeEmbedding(embeddings["a"], tagsByIssue["a"], tagNames, tagEmb,
		scoring.RidgeAnchorLambdaScored, scoring.RidgeAnchorLambdaUnscored)

	if len(query.Loading) != len(corpus.Loading) {
		t.Fatalf("loading dim mismatch: query %d corpus %d", len(query.Loading), len(corpus.Loading))
	}
	for k := range corpus.Loading {
		if math.Abs(query.Loading[k]-corpus.Loading[k]) > 1e-9 {
			t.Errorf("loading[%d]: query %f != corpus %f", k, query.Loading[k], corpus.Loading[k])
		}
	}
	if math.Abs(query.R2-corpus.R2) > 1e-9 {
		t.Errorf("R²: query %f != corpus %f", query.R2, corpus.R2)
	}
}
