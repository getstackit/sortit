package issuemath

import (
	"fmt"
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
	if !approx(mis.DriftCosine, 0.300) {
		t.Errorf("mistagged DriftCosine = %f, want ≈0.300", mis.DriftCosine)
	}
	misTags := tagsByName(mis.Tags)
	alpha := misTags["alpha"]
	if !alpha.Anchored {
		t.Error("alpha should be anchored on the mistagged issue")
	}
	if !approx(alpha.Delta, -0.6) {
		t.Errorf("mistagged alpha Δ = %f, want ≈−0.6 (spurious)", alpha.Delta)
	}
	beta := misTags["beta"]
	if beta.Anchored {
		t.Error("beta should be unanchored on the mistagged issue")
	}
	if !approx(beta.Delta, 1.0/1.05) {
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

// With orthonormal tag rows the closed form is Δ_scored = a/1.5 − 0.6 (tagged at
// relevance 0.9, λ=0.5) and Δ_noise = b/1.05 (untagged, λ=0.05), where a,b are
// the issue embedding's projections onto the two tags. This test builds a corpus
// where, on one target issue, the untagged "noise" tag has the larger raw |Δ|
// but far higher corpus variance, while the scored tag has a smaller raw |Δ| on
// a low-variance background — so standardizing flips which tag is the stronger
// candidate.
func TestComputeCorpusDrift_ZDeltaFlipsRawOrder(t *testing.T) {
	tagNames := []string{"scored", "noise"}
	tagEmb := map[string][]float64{
		"scored": {1, 0, 0, 0},
		"noise":  {0, 1, 0, 0},
	}
	const lambdaScored, lambdaUnscored = 0.5, 0.05

	items := make([]issues.Issue, 0, 23)
	issueEmb := make(map[string][]float64)
	add := func(id string, a, b float64) {
		items = append(items, issues.Issue{
			ID: id, Raw: id, Status: issues.StatusOpen,
			TagScores: []issues.TagRelevance{{Tag: "scored", Relevance: 0.9}},
		})
		issueEmb[id] = []float64{a, b, 0, 0}
	}
	// 22 background issues: scored well-tagged (a=0.9 → Δ_scored=0, low variance),
	// noise projection alternating ±0.8 (high variance, mean ≈ 0).
	for i := range 22 {
		b := 0.8
		if i%2 == 1 {
			b = -0.8
		}
		add(fmt.Sprintf("bg-%02d", i), 0.9, b)
	}
	// Target: scored genuinely over-claimed (a=0.3 → Δ_scored=−0.4), noise
	// projection large (b=0.9 → Δ_noise≈0.857) but ordinary for its wide spread.
	add("target", 0.3, 0.9)

	drifts := ComputeCorpusDrift(items, tagNames, issueEmb, tagEmb, lambdaScored, lambdaUnscored)

	var target IssueDrift
	for _, d := range drifts {
		if d.ID == "target" {
			target = d
		}
	}
	tags := tagsByName(target.Tags)
	scored, noise := tags["scored"], tags["noise"]

	// Raw deltas: noise has the larger magnitude, so by |Δ| it would rank first.
	if !approx(scored.Delta, -0.4) {
		t.Errorf("target scored Δ = %f, want ≈−0.4", scored.Delta)
	}
	if !approx(noise.Delta, 0.9/1.05) {
		t.Errorf("target noise Δ = %f, want ≈0.857", noise.Delta)
	}
	if math.Abs(noise.Delta) <= math.Abs(scored.Delta) {
		t.Fatalf("precondition: want |Δ_noise| (%f) > |Δ_scored| (%f)", noise.Delta, scored.Delta)
	}

	// Both tags have 23 observations, so both carry a z-score.
	if scored.ZDelta == nil || noise.ZDelta == nil {
		t.Fatalf("expected z-scores for both tags, got scored=%v noise=%v", scored.ZDelta, noise.ZDelta)
	}
	// The flip: standardized, the low-variance scored drift dominates the
	// high-variance noise swing even though its raw delta is smaller.
	if math.Abs(*scored.ZDelta) <= math.Abs(*noise.ZDelta) {
		t.Errorf("z-rule should flip order: |z_scored| (%f) must exceed |z_noise| (%f)", *scored.ZDelta, *noise.ZDelta)
	}
	if math.Abs(*scored.ZDelta) < DefaultDriftZDeltaThresholdSanity {
		t.Errorf("target scored |z| = %f, want a clearly-drifted magnitude", *scored.ZDelta)
	}
}

// DefaultDriftZDeltaThresholdSanity mirrors the diagnostics z-threshold (2.0)
// without importing that package; the flip test only needs to know the scored
// tag clears a "clearly outside the spread" bar.
const DefaultDriftZDeltaThresholdSanity = 2.0

func TestComputeCorpusDrift_ZDeltaSmallSample(t *testing.T) {
	tagNames := []string{"scored", "noise"}
	tagEmb := map[string][]float64{
		"scored": {1, 0, 0, 0},
		"noise":  {0, 1, 0, 0},
	}
	// Only 5 issues (< driftZDeltaMinObservations) → no z-score for any tag.
	items := make([]issues.Issue, 0, 5)
	issueEmb := make(map[string][]float64)
	for i := range 5 {
		id := fmt.Sprintf("i-%d", i)
		items = append(items, issues.Issue{
			ID: id, Status: issues.StatusOpen,
			TagScores: []issues.TagRelevance{{Tag: "scored", Relevance: 0.9}},
		})
		issueEmb[id] = []float64{0.3, 0.5 + 0.1*float64(i), 0, 0}
	}

	drifts := ComputeCorpusDrift(items, tagNames, issueEmb, tagEmb, 0.5, 0.05)
	if len(drifts) != 5 {
		t.Fatalf("expected 5 drifts, got %d", len(drifts))
	}
	for _, d := range drifts {
		for _, td := range d.Tags {
			if td.ZDelta != nil {
				t.Errorf("issue %s tag %s: want nil ZDelta below the observation floor, got %f", d.ID, td.Tag, *td.ZDelta)
			}
		}
	}
}

func TestComputeCorpusDrift_ZDeltaZeroVariance(t *testing.T) {
	tagNames := []string{"scored", "noise"}
	tagEmb := map[string][]float64{
		"scored": {1, 0, 0, 0},
		"noise":  {0, 1, 0, 0},
	}
	// 22 identical issues (≥ observation floor) → each tag's delta is constant
	// across the corpus → zero variance → no z-score despite the large sample.
	items := make([]issues.Issue, 0, 22)
	issueEmb := make(map[string][]float64)
	for i := range 22 {
		id := fmt.Sprintf("i-%02d", i)
		items = append(items, issues.Issue{
			ID: id, Status: issues.StatusOpen,
			TagScores: []issues.TagRelevance{{Tag: "scored", Relevance: 0.9}},
		})
		issueEmb[id] = []float64{0.3, 0.5, 0, 0}
	}

	drifts := ComputeCorpusDrift(items, tagNames, issueEmb, tagEmb, 0.5, 0.05)
	if len(drifts) != 22 {
		t.Fatalf("expected 22 drifts, got %d", len(drifts))
	}
	for _, d := range drifts {
		for _, td := range d.Tags {
			if td.ZDelta != nil {
				t.Errorf("issue %s tag %s: want nil ZDelta for zero-variance tag, got %f", d.ID, td.Tag, *td.ZDelta)
			}
		}
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) <= 1e-3 }

func tagsByName(tags []TagDrift) map[string]TagDrift {
	out := make(map[string]TagDrift, len(tags))
	for _, t := range tags {
		out[t.Tag] = t
	}
	return out
}
