package issuemap

import (
	"math"
	"testing"

	"splat/internal/issues"
	"splat/internal/scoring"
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
	for id, r2 := range decomp.IssueR2 {
		if r2 < 0.7 {
			t.Errorf("issue %s: expected high R2 for pure factor issue, got %f", id, r2)
		}
	}
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
	for id, r2 := range decomp.IssueR2 {
		if r2 != 0 {
			t.Errorf("issue %s: expected R2=0 for no-tag issue, got %f", id, r2)
		}
	}
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

	factor, residual := DecomposeEmbedding(emb, tags, tagNames, tagEmb, tagCov)

	if len(factor) != len(emb) {
		t.Fatalf("factor dimension %d != embedding dimension %d", len(factor), len(emb))
	}
	if len(residual) != len(emb) {
		t.Fatalf("residual dimension %d != embedding dimension %d", len(residual), len(emb))
	}

	// Factor and residual should be non-zero for tagged issue with residual.
	if isZeroVector(factor) {
		t.Error("factor embedding should not be zero for tagged issue")
	}
	if isZeroVector(residual) {
		t.Error("residual embedding should not be zero for issue with residual component")
	}
}

func TestDecomposeEmbedding_NoTags(t *testing.T) {
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
	}
	tagNames := []string{"alpha"}
	tagCov := buildTagCovariance(tagNames, tagEmb)

	emb := unitVec([]float64{0, 0, 1, 0})

	factor, residual := DecomposeEmbedding(emb, nil, tagNames, tagEmb, tagCov)

	if !isZeroVector(factor) {
		t.Error("factor should be zero for issue with no tags")
	}
	// Residual should be the original embedding (normalized).
	sim := dotProduct(residual, emb)
	if sim < 0.99 {
		t.Errorf("residual should approximate original embedding, similarity=%f", sim)
	}
}

func TestBlendFromDecomposition(t *testing.T) {
	decomp := FactorDecomposition{
		FactorWeight:   0.3,
		ResidualWeight: 0.7,
	}

	factorA := unitVec([]float64{1, 0, 0, 0})
	residualA := unitVec([]float64{0, 1, 0, 0})
	factorB := unitVec([]float64{1, 0, 0, 0})
	residualB := unitVec([]float64{0, 0, 1, 0}) // orthogonal residual

	factorSim, residualSim, blended := BlendFromDecomposition(decomp, factorA, residualA, factorB, residualB)

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
