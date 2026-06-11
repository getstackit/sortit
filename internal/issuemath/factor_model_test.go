package issuemath

import (
	"math"
	"testing"

	"sortit/internal/issues"
	"sortit/internal/scoring"
)

func TestComputeFactorDecomposition_PureFactorIssues(t *testing.T) {
	// When issue embeddings are linear combos of tag embeddings,
	// FactorWeight should approach MaxFactorWeight.
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
		"beta":  unitVec([]float64{0, 1, 0, 0}),
	}
	tagNames := []string{"alpha", "beta"}

	items := make([]issues.Issue, 10)
	embeds := make(map[string][]float64, 10)
	for i := range items {
		id := "issue-" + string(rune('A'+i))
		// Embedding is exactly the weighted sum of tag embeddings.
		w := float64(i+1) / 10.0
		emb := unitVec([]float64{w, 1 - w, 0, 0})
		items[i] = issues.Issue{
			ID:  id,
			Raw: "test issue " + id,
			TagScores: []issues.TagRelevance{
				{Tag: "alpha", Relevance: w},
				{Tag: "beta", Relevance: 1 - w},
			},
		}
		embeds[id] = emb
	}

	decomp := ComputeFactorDecomposition(items, tagNames, embeds, tagEmb)

	if decomp.FactorWeight < 0.8 {
		t.Errorf("expected high FactorWeight for pure factor issues, got %f", decomp.FactorWeight)
	}
	if decomp.FactorWeight > scoring.MaxFactorWeight {
		t.Errorf("FactorWeight %f exceeds MaxFactorWeight %f", decomp.FactorWeight, scoring.MaxFactorWeight)
	}
	if decomp.AggregateR2 < 0.8 {
		t.Errorf("expected high AggregateR2 for pure factor issues, got %f", decomp.AggregateR2)
	}
	decomp.AllR2(func(id string, r2 float64) {
		if r2 < 0.7 {
			t.Errorf("issue %s: expected high R2 for pure factor issue, got %f", id, r2)
		}
	})
}

func TestComputeFactorDecomposition_PureResidualIssues(t *testing.T) {
	// Issues with no tags — factor-predicted embedding is zero.
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
	}
	tagNames := []string{"alpha"}

	items := make([]issues.Issue, 10)
	embeds := make(map[string][]float64, 10)
	for i := range items {
		id := "issue-" + string(rune('A'+i))
		emb := unitVec([]float64{0, 0, float64(i + 1), float64(10 - i)})
		items[i] = issues.Issue{
			ID:        id,
			Raw:       "test issue " + id,
			TagScores: nil, // no tags
		}
		embeds[id] = emb
	}

	decomp := ComputeFactorDecomposition(items, tagNames, embeds, tagEmb)

	if decomp.FactorWeight > 0.1 {
		t.Errorf("expected low FactorWeight for no-tag issues, got %f", decomp.FactorWeight)
	}
	if decomp.FactorWeight < scoring.MinFactorWeight {
		t.Errorf("FactorWeight %f below MinFactorWeight %f", decomp.FactorWeight, scoring.MinFactorWeight)
	}
	if decomp.AggregateR2 > 0.01 {
		t.Errorf("expected near-zero AggregateR2 for no-tag issues, got %f", decomp.AggregateR2)
	}
	decomp.AllR2(func(id string, r2 float64) {
		if r2 != 0 {
			t.Errorf("issue %s: expected R2=0 for no-tag issue, got %f", id, r2)
		}
	})
}

func TestComputeFactorDecomposition_MixedVariance(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
		"beta":  unitVec([]float64{0, 1, 0, 0}),
	}
	tagNames := []string{"alpha", "beta"}

	items := make([]issues.Issue, 10)
	embeds := make(map[string][]float64, 10)
	for i := range items {
		id := "issue-" + string(rune('A'+i))
		// Mix of factor-explained and residual components.
		w := float64(i+1) / 10.0
		emb := unitVec([]float64{w, 1 - w, 0.5, 0.3})
		items[i] = issues.Issue{
			ID:  id,
			Raw: "test issue " + id,
			TagScores: []issues.TagRelevance{
				{Tag: "alpha", Relevance: w},
				{Tag: "beta", Relevance: 1 - w},
			},
		}
		embeds[id] = emb
	}

	decomp := ComputeFactorDecomposition(items, tagNames, embeds, tagEmb)

	if decomp.FactorWeight < scoring.MinFactorWeight || decomp.FactorWeight > scoring.MaxFactorWeight {
		t.Errorf("FactorWeight %f outside clamped range [%f, %f]",
			decomp.FactorWeight, scoring.MinFactorWeight, scoring.MaxFactorWeight)
	}
	if math.Abs(decomp.FactorWeight+decomp.ResidualWeight-1) > 1e-9 {
		t.Errorf("weights don't sum to 1: factor=%f residual=%f", decomp.FactorWeight, decomp.ResidualWeight)
	}
}

func TestComputeFactorDecomposition_TooFewIssues(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
	}
	tagNames := []string{"alpha"}

	items := []issues.Issue{
		{ID: "a", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}},
	}
	embeds := map[string][]float64{"a": unitVec([]float64{1, 0, 0, 0})}

	decomp := ComputeFactorDecomposition(items, tagNames, embeds, tagEmb)

	// Should fall back to hardcoded weights.
	if decomp.FactorWeight != scoring.FactorWeight {
		t.Errorf("expected fallback FactorWeight %f, got %f", scoring.FactorWeight, decomp.FactorWeight)
	}
	if decomp.Decomposed() {
		t.Fatal("expected fallback decomposition to be inactive")
	}
	if decomp.DecomposedCount() != 0 {
		t.Fatalf("expected no decomposed issues, got %d", decomp.DecomposedCount())
	}
}

func TestComputeFactorDecomposition_NoTags(t *testing.T) {
	items := make([]issues.Issue, 10)
	embeds := make(map[string][]float64, 10)
	for i := range items {
		id := "issue-" + string(rune('A'+i))
		items[i] = issues.Issue{ID: id, Raw: "test"}
		embeds[id] = unitVec([]float64{1, 0, 0, 0})
	}

	decomp := ComputeFactorDecomposition(items, nil, embeds, nil)

	if decomp.FactorWeight != scoring.FactorWeight {
		t.Errorf("expected fallback FactorWeight, got %f", decomp.FactorWeight)
	}
}

func TestComputeFactorDecomposition_EmptyEmbeddings(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
	}
	tagNames := []string{"alpha"}

	items := make([]issues.Issue, 10)
	embeds := make(map[string][]float64)
	for i := range items {
		id := "issue-" + string(rune('A'+i))
		items[i] = issues.Issue{
			ID:        id,
			Raw:       "test",
			TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
		}
		// No embeddings provided.
	}

	decomp := ComputeFactorDecomposition(items, tagNames, embeds, tagEmb)

	// No valid issues for decomposition — should fall back.
	if decomp.FactorWeight != scoring.FactorWeight {
		t.Errorf("expected fallback FactorWeight, got %f", decomp.FactorWeight)
	}
	if decomp.Decomposed() {
		t.Fatal("expected fallback decomposition to be inactive")
	}
}

func TestComputeFactorDecomposition_PartialValidBelowThresholdFallsBack(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
	}
	tagNames := []string{"alpha"}

	items := make([]issues.Issue, 6)
	embeds := make(map[string][]float64, 4)
	for i := range items {
		id := "issue-" + string(rune('A'+i))
		items[i] = issues.Issue{
			ID:        id,
			Raw:       "test",
			TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}},
		}
		if i < 4 {
			embeds[id] = unitVec([]float64{1, 0, 0, 0})
		}
	}

	decomp := ComputeFactorDecomposition(items, tagNames, embeds, tagEmb)

	if decomp.Decomposed() {
		t.Fatal("expected decomposition to fall back when valid issue count is below threshold")
	}
	if decomp.DecomposedCount() != 0 {
		t.Fatalf("expected 0 decomposed issues, got %d", decomp.DecomposedCount())
	}
	if factor := decomp.FactorEmbedding("issue-A"); factor != nil {
		t.Fatalf("expected no stored factor embedding on fallback, got %#v", factor)
	}
	if _, ok := decomp.IssueR2("issue-A"); ok {
		t.Fatal("expected no stored R2 on fallback")
	}
}

func TestDecomposeEmbedding_Consistency(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
		"beta":  unitVec([]float64{0, 1, 0, 0}),
	}
	tagNames := []string{"alpha", "beta"}
	tagCov := buildTagCovariance(tagNames, tagEmb)

	emb := unitVec([]float64{0.7, 0.3, 0.5, 0.1})
	tags := []issues.TagRelevance{
		{Tag: "alpha", Relevance: 0.8},
		{Tag: "beta", Relevance: 0.3},
	}

	dv := DecomposeEmbedding(emb, tags, tagNames, tagEmb, tagCov)

	if len(dv.Factor) != len(emb) {
		t.Fatalf("factor dimension %d != embedding dimension %d", len(dv.Factor), len(emb))
	}
	if len(dv.Residual) != len(emb) {
		t.Fatalf("residual dimension %d != embedding dimension %d", len(dv.Residual), len(emb))
	}

	// Factor and residual should be non-zero for tagged issue with residual.
	if isZeroVector(dv.Factor) {
		t.Error("factor embedding should not be zero for tagged issue")
	}
	if isZeroVector(dv.Residual) {
		t.Error("residual embedding should not be zero for issue with residual component")
	}
	if dv.FactorNorm <= 0 {
		t.Errorf("factor norm should be positive for an aligned tagged embedding, got %f", dv.FactorNorm)
	}
	if dv.ResidualNorm <= 0 {
		t.Errorf("residual norm should be positive for an embedding with residual, got %f", dv.ResidualNorm)
	}
	// Norms reconstruct the unit embedding's energy: f² + r² = 1.
	energy := dv.FactorNorm*dv.FactorNorm + dv.ResidualNorm*dv.ResidualNorm
	if math.Abs(energy-1) > 1e-9 {
		t.Errorf("factorNorm²+residualNorm² = %f, want 1 for a unit embedding", energy)
	}
}

func TestDecomposeEmbedding_NoTags(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
	}
	tagNames := []string{"alpha"}
	tagCov := buildTagCovariance(tagNames, tagEmb)

	emb := []float64{0, 0, 3, 4}

	dv := DecomposeEmbedding(emb, nil, tagNames, tagEmb, tagCov)

	if !isZeroVector(dv.Factor) {
		t.Error("factor should be zero for issue with no tags")
	}
	// Residual should preserve direction while being normalized for similarity use.
	sim := dotProduct(dv.Residual, unitVec(emb))
	if sim < 0.99 {
		t.Errorf("residual should approximate original embedding, similarity=%f", sim)
	}
	if math.Abs(dotProduct(dv.Residual, dv.Residual)-1) > 1e-9 {
		t.Errorf("residual should be unit normalized, got squared norm=%f", dotProduct(dv.Residual, dv.Residual))
	}
	// The (unnormalized) embedding has magnitude 5; the norm should keep it.
	if math.Abs(dv.ResidualNorm-5) > 1e-9 {
		t.Errorf("residual norm should be the embedding magnitude 5, got %f", dv.ResidualNorm)
	}
	if dv.FactorNorm != 0 {
		t.Errorf("factor norm should be zero with no tags, got %f", dv.FactorNorm)
	}
}

func TestBlendFromDecomposition(t *testing.T) {
	decomp := FactorDecomposition{
		FactorWeight:   0.3,
		ResidualWeight: 0.7,
	}

	a := DecomposedEmbedding{
		Factor:       unitVec([]float64{1, 0, 0, 0}),
		Residual:     unitVec([]float64{0, 1, 0, 0}),
		FactorNorm:   0.8,
		ResidualNorm: 0.6,
	}
	b := DecomposedEmbedding{
		Factor:       unitVec([]float64{1, 0, 0, 0}),
		Residual:     unitVec([]float64{0, 0, 1, 0}), // orthogonal residual
		FactorNorm:   0.8,
		ResidualNorm: 0.6,
	}

	factorSim, residualSim, blended := BlendFromDecomposition(decomp, a, b)

	if math.Abs(factorSim-1.0) > 1e-9 {
		t.Errorf("expected factorSim=1.0, got %f", factorSim)
	}
	if math.Abs(residualSim) > 1e-9 {
		t.Errorf("expected residualSim=0.0, got %f", residualSim)
	}
	expected := 0.3*1.0 + 0.7*0.0
	if math.Abs(blended-expected) > 1e-9 {
		t.Errorf("expected blended=%f, got %f", expected, blended)
	}
}

// TestBlendFromDecomposition_ResidualMagnitudeInvariant pins a deliberate,
// measured decision: residual similarity compares directions only. Scaling
// by residual magnitudes was tried and regressed fixture NDCG@8 (see the
// BlendFromDecomposition comment) — a well-explained issue's residual
// direction is its discriminating content, not noise. If this test breaks
// because someone reintroduces magnitude scaling, the change must come with
// fresh harness evidence.
func TestBlendFromDecomposition_ResidualMagnitudeInvariant(t *testing.T) {
	decomp := FactorDecomposition{FactorWeight: 0, ResidualWeight: 1}

	dir := unitVec([]float64{0, 1, 0, 0})
	small := DecomposedEmbedding{Factor: unitVec([]float64{1, 0, 0, 0}), Residual: dir, FactorNorm: 0.99, ResidualNorm: 0.1}
	large := DecomposedEmbedding{Factor: unitVec([]float64{1, 0, 0, 0}), Residual: dir, FactorNorm: 0.2, ResidualNorm: 0.98}

	_, smallSim, _ := BlendFromDecomposition(decomp, small, small)
	_, largeSim, _ := BlendFromDecomposition(decomp, large, large)

	if math.Abs(smallSim-1) > 1e-9 || math.Abs(largeSim-1) > 1e-9 {
		t.Errorf("residual similarity should be magnitude-invariant, got small=%f large=%f", smallSim, largeSim)
	}
}

// TestComputeFactorDecomposition_AntiAlignedEmbedding verifies the
// non-negative projection rule: an embedding pointing against its own tag
// direction yields no factor evidence (zero factor, R² = 0) instead of a
// sign-flipped factor that would score ≈ −1 against same-tagged issues.
func TestComputeFactorDecomposition_AntiAlignedEmbedding(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
		"beta":  unitVec([]float64{0, 1, 0, 0}),
	}
	tagNames := []string{"alpha", "beta"}

	items := make([]issues.Issue, 0, 6)
	embeds := make(map[string][]float64, 6)
	// Five aligned issues to satisfy MinDecompositionIssues, one anti-aligned.
	for i := range 5 {
		id := "aligned-" + string(rune('A'+i))
		items = append(items, issues.Issue{ID: id, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}}})
		embeds[id] = unitVec([]float64{1, 0.2, 0.1, 0})
	}
	items = append(items, issues.Issue{ID: "anti", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}}})
	embeds["anti"] = unitVec([]float64{-1, 0, 0.1, 0})

	decomp := ComputeFactorDecomposition(items, tagNames, embeds, tagEmb)
	if !decomp.Decomposed() {
		t.Fatal("expected decomposition to run")
	}

	dv, ok := decomp.DecomposedFor("anti")
	if !ok {
		t.Fatal("anti-aligned issue should still be decomposed")
	}
	if !isZeroVector(dv.Factor) {
		t.Error("anti-aligned issue should have a zero factor, not a sign-flipped one")
	}
	if dv.FactorNorm != 0 {
		t.Errorf("anti-aligned issue should have factor norm 0, got %f", dv.FactorNorm)
	}
	if r2, _ := decomp.IssueR2("anti"); r2 != 0 {
		t.Errorf("anti-aligned issue should have R² = 0, got %f", r2)
	}

	aligned, _ := decomp.DecomposedFor("aligned-A")
	if isZeroVector(aligned.Factor) {
		t.Error("aligned issue should keep its factor component")
	}
}

func TestComputeFactorDecomposition_DimensionMismatch(t *testing.T) {
	// Tag embeddings have dim 4, issue embeddings have dim 3 — should skip.
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
	}
	tagNames := []string{"alpha"}

	items := make([]issues.Issue, 6)
	embeds := make(map[string][]float64, 6)
	for i := range items {
		id := "issue-" + string(rune('A'+i))
		items[i] = issues.Issue{
			ID:        id,
			Raw:       "test",
			TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
		}
		embeds[id] = unitVec([]float64{1, 0, 0}) // dim 3, mismatches tag dim 4
	}

	decomp := ComputeFactorDecomposition(items, tagNames, embeds, tagEmb)

	// Should fall back to hardcoded weights (no valid decompositions).
	if decomp.FactorWeight != scoring.FactorWeight {
		t.Errorf("expected fallback FactorWeight for dim mismatch, got %f", decomp.FactorWeight)
	}
	if decomp.Decomposed() {
		t.Fatal("expected decomposition to be inactive on dimension mismatch")
	}
}

func TestComputeFactorDecomposition_PreservesRawNorms(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
	}
	tagNames := []string{"alpha"}

	items := []issues.Issue{
		{ID: "pure-a", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}},
		{ID: "mixed-a", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}},
		{ID: "mixed-b", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}},
		{ID: "mixed-c", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}},
		{ID: "mixed-d", Raw: "test", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}},
	}
	embeds := map[string][]float64{
		"pure-a":  unitVec([]float64{1, 0, 0, 0}),
		"mixed-a": unitVec([]float64{1, 0, 1, 0}),
		"mixed-b": unitVec([]float64{1, 0, 1, 0}),
		"mixed-c": unitVec([]float64{1, 0, 1, 0}),
		"mixed-d": unitVec([]float64{1, 0, 1, 0}),
	}

	decomp := ComputeFactorDecomposition(items, tagNames, embeds, tagEmb)
	if !decomp.Decomposed() {
		t.Fatal("expected decomposition to be active")
	}
	wantHalf := math.Sqrt(0.5)

	factorNorm, ok := decomp.FactorNorm("pure-a")
	if !ok || math.Abs(factorNorm-1) > 1e-9 {
		t.Fatalf("expected pure issue explained norm of 1, got %f (ok=%v)", factorNorm, ok)
	}
	residualNorm, ok := decomp.ResidualNorm("pure-a")
	if !ok || math.Abs(residualNorm) > 1e-9 {
		t.Fatalf("expected pure issue residual norm of 0, got %f (ok=%v)", residualNorm, ok)
	}

	factorNorm, ok = decomp.FactorNorm("mixed-a")
	if !ok || math.Abs(factorNorm-wantHalf) > 1e-9 {
		t.Fatalf("expected mixed issue explained norm of sqrt(0.5), got %f (ok=%v)", factorNorm, ok)
	}
	residualNorm, ok = decomp.ResidualNorm("mixed-a")
	if !ok || math.Abs(residualNorm-wantHalf) > 1e-9 {
		t.Fatalf("expected mixed issue residual norm of sqrt(0.5), got %f (ok=%v)", residualNorm, ok)
	}
}

// unitVec normalizes a vector to unit length.
func unitVec(v []float64) []float64 {
	out := make([]float64, len(v))
	copy(out, v)
	var mag float64
	for _, val := range out {
		mag += val * val
	}
	if mag > 0 {
		mag = math.Sqrt(mag)
		for i := range out {
			out[i] /= mag
		}
	}
	return out
}
