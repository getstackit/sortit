package matheval

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"sortit/internal/issuemath"
	"sortit/internal/issues"
)

// This file holds the map-projection eval (WP-303): it drives the current
// production projection (issuemath.ComputePositionsAligned, PCA over tag
// relevance X smeared by the tag covariance Σ) over the fixture corpus and
// scores the resulting 2-D layout on three complementary metrics — neighborhood
// preservation, cluster legibility, and refresh stability. The layout is a
// single artifact: it is computed ONCE over the synthetic fixture's tag
// geometry (corpus.json), so every metric is fixture-independent EXCEPT the two
// embedding-reference neighborhood rows, which hold the layout fixed and change
// only the reference space (synthetic 24-d vs real 1536-d centered cosine). The
// map baseline lives beside the ranking baselines in testdata/baseline.json
// under the top-level `map` key and is asserted on every ordinary `go test` run.
//
// The projection consumes issue-tag relevance, not embeddings, so a modest
// overlap against the real 1536-d embedding neighborhoods is the honest ceiling
// of a tag-based layout, not a defect — this WP measures, it does not tune the
// projection (positions math is unchanged; see internal/issuemath).

// MapBaseline guards the current map projection on the three WP-303 metrics.
// Layout metrics are computed once over the synthetic fixture; Neighborhood
// carries the two embedding-reference geometries separately.
type MapBaseline struct {
	Issues       int                  `json:"issues"`
	Neighborhood NeighborhoodBaseline `json:"neighborhood"`
	Silhouette   float64              `json:"silhouette"`
	Stability    StabilityBaseline    `json:"stability"`
}

// NeighborhoodBaseline holds the neighborhood-preservation scores at k ∈ {5,10}.
// GroundTruthDomain is the fraction of layout-kNN sharing the issue's primary
// generator domain (does the map respect the intended regions?). EmbeddingSynthetic
// and EmbeddingReal are symmetric kNN overlap against centered embedding cosine
// on the two fixture geometries (does the map respect embedding semantics?) — the
// layout is identical for both; only the reference neighborhoods differ.
type NeighborhoodBaseline struct {
	GroundTruthDomain  KNNPair `json:"groundTruthDomain"`
	EmbeddingSynthetic KNNPair `json:"embeddingSynthetic"`
	EmbeddingReal      KNNPair `json:"embeddingReal"`
}

// KNNPair is one neighborhood metric at the two evaluated neighborhood sizes.
type KNNPair struct {
	K5  float64 `json:"k5"`
	K10 float64 `json:"k10"`
}

// StabilityBaseline summarizes per-issue displacement over the deterministic
// mutation sequence. Displacements are in the [0.05,0.95] layout units; lower is
// better (a stable, Procrustes-aligned projection barely moves surviving issues
// when the corpus is perturbed).
type StabilityBaseline struct {
	Steps            int     `json:"steps"`
	MeanDisplacement float64 `json:"meanDisplacement"`
	P95Displacement  float64 `json:"p95Displacement"`
}

// stabilityTolerance bounds float/platform noise in the displacement metric.
// Positions span [0.05,0.95] (0.9 wide), so 0.01 is ~1% of the map; a real
// continuity regression moves surviving issues far more than that.
const stabilityTolerance = 0.01

// computeMapMetrics builds the current projection over the synthetic fixture and
// scores all three WP-303 metrics. The real corpus supplies only the real-model
// embedding-reference neighborhood (metric 1a, second row); the layout, the
// ground-truth-domain neighborhood, the silhouette, and the stability sweep are
// all fixture-independent, computed on the synthetic geometry.
func computeMapMetrics(t *testing.T, synthetic, real Corpus) MapBaseline {
	t.Helper()

	now := time.Now().UTC()
	storeIssues := synthetic.StoreIssues(now)
	tagNames := synthetic.TagNames()
	centeredTags := centeredProjectionTags(synthetic)

	positions, err := issuemath.ComputePositionsAligned(storeIssues, tagNames, centeredTags, nil)
	if err != nil {
		t.Fatalf("compute projection positions: %v", err)
	}

	ids := sortedIssueIDs(storeIssues)
	domainOf := issuePrimaryDomains(synthetic)

	layoutK5 := layoutNeighbors(positions, ids, 5)
	layoutK10 := layoutNeighbors(positions, ids, 10)

	synEmb := centeredIssueEmbeddings(synthetic)
	realEmb := centeredIssueEmbeddings(real)

	neighborhood := NeighborhoodBaseline{
		GroundTruthDomain: KNNPair{
			K5:  round4(meanDomainPurity(ids, layoutK5, domainOf)),
			K10: round4(meanDomainPurity(ids, layoutK10, domainOf)),
		},
		EmbeddingSynthetic: KNNPair{
			K5:  round4(meanKNNOverlap(ids, layoutK5, embeddingNeighbors(synEmb, ids, 5), 5)),
			K10: round4(meanKNNOverlap(ids, layoutK10, embeddingNeighbors(synEmb, ids, 10), 10)),
		},
		EmbeddingReal: KNNPair{
			K5:  round4(meanKNNOverlap(ids, layoutK5, embeddingNeighbors(realEmb, ids, 5), 5)),
			K10: round4(meanKNNOverlap(ids, layoutK10, embeddingNeighbors(realEmb, ids, 10), 10)),
		},
	}

	silhouette := round4(silhouetteByLabel(positions, domainOf))
	stability := computeStability(t, storeIssues, tagNames, centeredTags)

	return MapBaseline{
		Issues:       len(storeIssues),
		Neighborhood: neighborhood,
		Silhouette:   silhouette,
		Stability:    stability,
	}
}

// stabilityMutationSteps is the length of the deterministic refresh sequence
// (the WP-302/306 add / tweak / remove pattern, replicated locally over the
// matheval corpus rather than importing the themes soak internals).
const stabilityMutationSteps = 12

// computeStability runs the deterministic 12-step mutation sequence, recomputing
// the layout each step WITH the production Procrustes alignment chain
// (ComputePositionsAligned against the previous layout) and pooling the per-issue
// displacements of surviving issues across all steps. The three mutations cycle:
// add a cloned issue, tweak a tag score, remove the newest clone.
func computeStability(t *testing.T, base []issues.Issue, tagNames []string, centeredTags map[string][]float64) StabilityBaseline {
	t.Helper()

	working := append([]issues.Issue(nil), base...)
	prev, err := issuemath.ComputePositionsAligned(working, tagNames, centeredTags, nil)
	if err != nil {
		t.Fatalf("stability: initial projection: %v", err)
	}

	pooled := make([]float64, 0, stabilityMutationSteps*len(base))
	added := 0
	for step := range stabilityMutationSteps {
		switch step % 3 {
		case 0: // add a cloned issue: a fresh ID over an existing issue's tags/embedding
			src := working[step%len(base)]
			clone := src
			clone.ID = fmt.Sprintf("stability-clone-%02d", added)
			clone.TagScores = append([]issues.TagRelevance(nil), src.TagScores...)
			clone.Embedding = append([]float64(nil), src.Embedding...)
			working = append(working, clone)
			added++
		case 1: // tweak a tag score on an existing issue (fresh slice, base untouched)
			for off := range working {
				j := (step + off) % len(working)
				if len(working[j].TagScores) > 0 {
					ts := append([]issues.TagRelevance(nil), working[j].TagScores...)
					ts[0].Relevance = clampRelevance(ts[0].Relevance*0.9 + 0.05)
					working[j].TagScores = ts
					break
				}
			}
		case 2: // remove the newest issue (always a previously added clone)
			if len(working) > len(base) {
				working = working[:len(working)-1]
			}
		}

		cur, err := issuemath.ComputePositionsAligned(working, tagNames, centeredTags, prev)
		if err != nil {
			t.Fatalf("stability: projection at step %d: %v", step, err)
		}
		pooled = append(pooled, layoutDisplacements(prev, cur)...)
		prev = cur
	}

	mean, p95 := meanAndP95(pooled)
	return StabilityBaseline{
		Steps:            stabilityMutationSteps,
		MeanDisplacement: round4(mean),
		P95Displacement:  round4(p95),
	}
}

// clampRelevance keeps a mutated tag relevance in the fixture's valid range.
func clampRelevance(v float64) float64 {
	if v < 0.05 {
		return 0.05
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}

// centeredProjectionTags reproduces the tag-embedding centering production
// applies before the projection (runtimeMapInputs → CenterEmbeddings): the tag
// covariance Σ that smears the PCA loadings is built from these centered tag
// cosines. Only the tag embeddings feed the layout; the returned issue centering
// is discarded.
func centeredProjectionTags(corpus Corpus) map[string][]float64 {
	_, centeredTags, _ := issuemath.CenterEmbeddings(corpus.IssueEmbeddings(), corpus.TagEmbeddings())
	return centeredTags
}

// centeredIssueEmbeddings returns the corpus issue embeddings centered against
// the corpus means (whitepaper §2.0), the geometry the similarity math uses —
// the reference space for the embedding neighborhood-preservation metric.
func centeredIssueEmbeddings(corpus Corpus) map[string][]float64 {
	centered, _, _ := issuemath.CenterEmbeddings(corpus.IssueEmbeddings(), corpus.TagEmbeddings())
	return centered
}

// issuePrimaryDomains maps each issue ID to its primary ground-truth domain
// (the domain of its highest-relevance non-generic tag), reusing the same
// derivation the explore/person fixtures pin (derived_fixtures_test.go). Empty
// for the untagged off-taxonomy notes.
func issuePrimaryDomains(corpus Corpus) map[string]string {
	domainOf, generic := tagDomainOf(), tagIsGeneric()
	out := make(map[string]string, len(corpus.Issues))
	for _, issue := range corpus.Issues {
		out[issue.ID] = primaryDomain(issueDomainWeights(issue, domainOf, generic))
	}
	return out
}

// sortedIssueIDs returns the issue IDs in sorted order for deterministic metrics.
func sortedIssueIDs(storeIssues []issues.Issue) []string {
	ids := make([]string, len(storeIssues))
	for i, issue := range storeIssues {
		ids[i] = issue.ID
	}
	slices.Sort(ids)
	return ids
}

// assertMap asserts the map projection metrics against baseline. The three
// quality metrics (neighborhood overlaps, silhouette) must not regress below
// baseline; the stability displacements must not rise above it.
func assertMap(t *testing.T, got, want MapBaseline) {
	t.Helper()
	if got.Issues != want.Issues {
		t.Errorf("[map] issue count changed: %d, baseline %d — rerun with -update", got.Issues, want.Issues)
	}

	assertNoRegression(t, "map neighborhood domain k5", got.Neighborhood.GroundTruthDomain.K5, want.Neighborhood.GroundTruthDomain.K5)
	assertNoRegression(t, "map neighborhood domain k10", got.Neighborhood.GroundTruthDomain.K10, want.Neighborhood.GroundTruthDomain.K10)
	assertNoRegression(t, "map neighborhood embedding-synthetic k5", got.Neighborhood.EmbeddingSynthetic.K5, want.Neighborhood.EmbeddingSynthetic.K5)
	assertNoRegression(t, "map neighborhood embedding-synthetic k10", got.Neighborhood.EmbeddingSynthetic.K10, want.Neighborhood.EmbeddingSynthetic.K10)
	assertNoRegression(t, "map neighborhood embedding-real k5", got.Neighborhood.EmbeddingReal.K5, want.Neighborhood.EmbeddingReal.K5)
	assertNoRegression(t, "map neighborhood embedding-real k10", got.Neighborhood.EmbeddingReal.K10, want.Neighborhood.EmbeddingReal.K10)
	assertNoRegression(t, "map silhouette", got.Silhouette, want.Silhouette)

	if got.Stability.Steps != want.Stability.Steps {
		t.Errorf("[map] stability steps changed: %d, baseline %d — rerun with -update", got.Stability.Steps, want.Stability.Steps)
	}
	assertNoIncrease(t, "map stability meanDisplacement", got.Stability.MeanDisplacement, want.Stability.MeanDisplacement)
	assertNoIncrease(t, "map stability p95Displacement", got.Stability.P95Displacement, want.Stability.P95Displacement)
}

// assertNoIncrease fails when a cost metric (lower is better) rises above the
// baseline — the stability mirror of assertNoRegression.
func assertNoIncrease(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got > want+stabilityTolerance {
		t.Errorf("%s worsened: got %.4f, baseline %.4f — layout continuity regressed, or rerun with -update if intentional", name, got, want)
	} else if got < want-stabilityTolerance {
		t.Logf("%s improved: got %.4f, baseline %.4f — consider ratcheting the baseline with -update", name, got, want)
	}
}

// logMap prints the map projection metrics under -v.
func logMap(t *testing.T, m MapBaseline) {
	t.Helper()
	t.Logf("[map] projection over %d issues (layout computed once on the synthetic fixture)", m.Issues)
	t.Logf("[map] neighborhood domain purity: k5=%.4f k10=%.4f", m.Neighborhood.GroundTruthDomain.K5, m.Neighborhood.GroundTruthDomain.K10)
	t.Logf("[map] neighborhood embedding-overlap synthetic: k5=%.4f k10=%.4f", m.Neighborhood.EmbeddingSynthetic.K5, m.Neighborhood.EmbeddingSynthetic.K10)
	t.Logf("[map] neighborhood embedding-overlap real: k5=%.4f k10=%.4f", m.Neighborhood.EmbeddingReal.K5, m.Neighborhood.EmbeddingReal.K10)
	t.Logf("[map] silhouette (ground-truth domains): %.4f", m.Silhouette)
	t.Logf("[map] refresh stability over %d steps: meanDisp=%.4f p95Disp=%.4f", m.Stability.Steps, m.Stability.MeanDisplacement, m.Stability.P95Displacement)
}
