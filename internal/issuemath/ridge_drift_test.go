package issuemath

import (
	"math"
	"testing"

	"sortit/internal/issues"
)

// With orthonormal tag rows the gram T Tᵀ is the identity, so the ridge solve
// decouples per tag to f_k = ((T e)_k + λ_k r_k) / (1 + λ_k). That closed form
// lets these tests assert exact loadings, deltas, and DriftCosine.
func TestComputeCorpusDrift_MisTagAttribution(t *testing.T) {
	tagNames := []string{"alpha", "beta"}
	tagEmb := map[string][]float64{
		"alpha": unitVec([]float64{1, 0, 0, 0}),
		"beta":  unitVec([]float64{0, 1, 0, 0}),
	}
	const lambdaScored, lambdaUnscored = 0.5, 0.05

	items := []issues.Issue{
		// Tagged "alpha" but the embedding points at "beta": alpha is
		// over-claimed (spurious), beta is missing.
		{ID: "mistagged", Raw: "m", Status: issues.StatusOpen,
			TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}}},
		// Tagged "alpha" and the embedding agrees: low drift.
		{ID: "welltagged", Raw: "w", Status: issues.StatusOpen,
			TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}}},
		// No tags in the catalog → no anchor to disagree with → excluded.
		{ID: "untagged", Raw: "u", Status: issues.StatusOpen},
		// No embedding → excluded.
		{ID: "noembed", Raw: "n", Status: issues.StatusOpen,
			TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}}},
	}
	issueEmb := map[string][]float64{
		"mistagged":  unitVec([]float64{0, 1, 0, 0}),
		"welltagged": unitVec([]float64{1, 0, 0, 0}),
		"untagged":   unitVec([]float64{0, 0, 1, 0}),
		// "noembed" intentionally absent.
	}

	drifts := ComputeCorpusDrift(items, tagNames, issueEmb, tagEmb, lambdaScored, lambdaUnscored)

	byID := make(map[string]IssueDrift, len(drifts))
	for _, d := range drifts {
		byID[d.ID] = d
	}
	if len(drifts) != 2 {
		t.Fatalf("expected only mistagged+welltagged, got %d: %+v", len(drifts), drifts)
	}
	if _, ok := byID["untagged"]; ok {
		t.Fatal("untagged issue (no anchor) must be excluded")
	}
	if _, ok := byID["noembed"]; ok {
		t.Fatal("issue without an embedding must be excluded")
	}

	// Mis-tagged: DriftCosine ≈ 0.300, alpha spurious (Δ ≈ −0.6), beta missing (Δ ≈ 0.952).
	mis := byID["mistagged"]
	if !approx(mis.DriftCosine, 0.300, 1e-3) {
		t.Errorf("mistagged DriftCosine = %f, want ≈0.300", mis.DriftCosine)
	}
	misTags := tagsByName(mis.Tags)
	alpha := misTags["alpha"]
	if !alpha.Anchored {
		t.Error("alpha should be anchored on the mistagged issue")
	}
	if !approx(alpha.Delta, -0.6, 1e-3) {
		t.Errorf("mistagged alpha Δ = %f, want ≈−0.6 (spurious)", alpha.Delta)
	}
	beta := misTags["beta"]
	if beta.Anchored {
		t.Error("beta should be unanchored on the mistagged issue")
	}
	if !approx(beta.Delta, 1.0/1.05, 1e-3) {
		t.Errorf("mistagged beta Δ = %f, want ≈0.952 (missing)", beta.Delta)
	}

	// Well-tagged: geometry agrees → DriftCosine ≈ 1.0, alpha Δ small, no beta row.
	well := byID["welltagged"]
	if well.DriftCosine < 0.95 {
		t.Errorf("welltagged DriftCosine = %f, want ≈1.0", well.DriftCosine)
	}
	wellTags := tagsByName(well.Tags)
	if math.Abs(wellTags["alpha"].Delta) > 0.1 {
		t.Errorf("welltagged alpha Δ = %f, want ≈0", wellTags["alpha"].Delta)
	}
	if _, ok := wellTags["beta"]; ok {
		t.Errorf("welltagged should have no beta row (f_beta=0, anchor=0), got %+v", wellTags["beta"])
	}
}

func TestComputeCorpusDrift_EmptyInputs(t *testing.T) {
	tagEmb := map[string][]float64{"alpha": unitVec([]float64{1, 0, 0, 0})}
	items := []issues.Issue{{ID: "a", TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 1}}}}

	if got := ComputeCorpusDrift(nil, []string{"alpha"}, map[string][]float64{}, tagEmb, 0.5, 0.05); got != nil {
		t.Errorf("no items: want nil, got %+v", got)
	}
	if got := ComputeCorpusDrift(items, nil, map[string][]float64{}, tagEmb, 0.5, 0.05); got != nil {
		t.Errorf("no tags: want nil, got %+v", got)
	}
	// Tag embeddings with no dimension → nil.
	if got := ComputeCorpusDrift(items, []string{"alpha"}, map[string][]float64{}, map[string][]float64{}, 0.5, 0.05); got != nil {
		t.Errorf("no tag embeddings: want nil, got %+v", got)
	}
}

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func tagsByName(tags []TagDrift) map[string]TagDrift {
	out := make(map[string]TagDrift, len(tags))
	for _, t := range tags {
		out[t.Tag] = t
	}
	return out
}
