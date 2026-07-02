package matheval

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"sortit/internal/issuemath"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
	"sortit/internal/scoring"
)

const (
	judgmentsPath = "testdata/judgments.json"
	baselinePath  = "testdata/baseline.json"

	// evalK matches the default result limit users actually see.
	evalK = scoring.DefaultResultLimit

	// rankingTolerance absorbs float noise in NDCG/Recall comparisons;
	// anything below baseline−tolerance is a regression.
	rankingTolerance = 0.005
	// r2Tolerance bounds drift of the R² distribution in either direction —
	// the distribution is a fingerprint of the decomposition math, so any
	// real movement should be acknowledged by updating the baseline.
	r2Tolerance = 0.02
)

// Baseline is the golden metrics file. It keys the metrics by fixture:
// `synthetic` (embeddings synthesized from tag sums, the original harness) and
// `real` (the same corpus/judgments re-embedded with the production
// text-embedding-3-small model, WP-301). Every ordinary `go test` run asserts
// both fixtures with no network, using the committed vectors. Regenerate after
// an intentional math change with `go test ./internal/matheval -update`.
type Baseline struct {
	Synthetic FixtureBaseline `json:"synthetic"`
	Real      FixtureBaseline `json:"real"`
}

// FixtureBaseline guards every surface on one fixture. The top-level `rank1`
// and `ridge` are the SEARCH path (rank-1 fallback and the ridge tag-space
// default injected via WithRidgeSimilarity) with their factor-model summaries;
// their values are frozen by the WP-101/301 guards. `explore` and `person`
// (WP-302) add the two derived surfaces that inherited the ridge default without
// coverage, each on both models.
type FixtureBaseline struct {
	Rank1   PathMetrics     `json:"rank1"`
	Ridge   PathMetrics     `json:"ridge"`
	Explore SurfaceBaseline `json:"explore"`
	Person  SurfaceBaseline `json:"person"`
}

// PathMetrics is one similarity model's golden numbers. GCVLambdaUnscored is
// recorded for the ridge path only, for observability — it is re-derived each
// run and never asserted, so a grid or GCV change surfaces as a search-metric
// delta rather than silent λ drift.
type PathMetrics struct {
	Search            SearchMetrics      `json:"search"`
	FactorModel       FactorModelMetrics `json:"factorModel"`
	GCVLambdaUnscored float64            `json:"gcvLambdaUnscored,omitempty"`
}

// SearchMetrics aggregates ranking quality over the judgment set.
type SearchMetrics struct {
	Queries int     `json:"queries"`
	NDCG    float64 `json:"ndcg@8"`
	Recall  float64 `json:"recall@8"`
}

// FactorModelMetrics summarizes the tag-factor decomposition over the corpus.
type FactorModelMetrics struct {
	FactorWeight float64      `json:"factorWeight"`
	AggregateR2  float64      `json:"aggregateR2"`
	R2           Distribution `json:"r2"`
}

// TestMathEval is the offline evaluation harness (whitepaper §10.2, item 9).
// It drives SearchFromQueryWithTags end-to-end over both fixtures — `synthetic`
// (tag-sum embeddings) and `real` (production text-embedding-3-small vectors
// over the same texts, WP-301) — for every labeled query on both similarity
// models — the rank-1 fallback and the ridge tag-space default
// (WithRidgeSimilarity at the GCV-selected penalty) — summarizes per-issue R²
// for each, and compares all four entries against the recorded baseline. No
// network: the real fixture uses committed vectors. Run with -v to see the
// metrics; see docs/math-eval.md for interpretation.
func TestMathEval(t *testing.T) {
	syntheticCorpus, realCorpus := loadFixtures(t)
	got := Baseline{
		Synthetic: computeFixtureMetrics(t, syntheticCorpus),
		Real:      computeFixtureMetrics(t, realCorpus),
	}

	logFixture(t, "synthetic", got.Synthetic)
	logFixture(t, "real", got.Real)

	if *update {
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatalf("marshal baseline: %v", err)
		}
		if err := os.WriteFile(baselinePath, append(data, '\n'), 0o644); err != nil {
			t.Fatalf("write %s: %v", baselinePath, err)
		}
		t.Logf("recorded baseline in %s", baselinePath)
		return
	}

	data, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read %s (run `go test ./internal/matheval -update` to record a baseline): %v", baselinePath, err)
	}
	var want Baseline
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("parse %s: %v", baselinePath, err)
	}

	assertFixture(t, "synthetic", got.Synthetic, want.Synthetic)
	assertFixture(t, "real", got.Real, want.Real)
}

// loadFixtures loads the synthetic corpus and, overlaid on it, the real-model
// corpus (same texts/judgments, committed text-embedding-3-small vectors).
func loadFixtures(t *testing.T) (synthetic, real Corpus) {
	t.Helper()
	synthetic, err := LoadCorpus(corpusPath)
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	re, err := LoadRealEmbeddings(realEmbeddingsPath)
	if err != nil {
		t.Fatalf("load real embeddings (run `go run ./internal/matheval/cmd/embedfixture` to regenerate): %v", err)
	}
	real, err = synthetic.RealCorpus(re)
	if err != nil {
		t.Fatalf("build real corpus: %v", err)
	}
	return synthetic, real
}

// logFixture prints one fixture's search, explore, and person metrics under -v.
func logFixture(t *testing.T, fixture string, fb FixtureBaseline) {
	t.Helper()
	logPath(t, fixture+"/rank1", fb.Rank1)
	logPath(t, fixture+"/ridge", fb.Ridge)
	t.Logf("[%s] ridge GCV λ_unscored = %.4f", fixture, fb.Ridge.GCVLambdaUnscored)
	logSurface(t, fixture+"/explore", fb.Explore)
	logSurface(t, fixture+"/person", fb.Person)
}

// logSurface prints one derived surface's rank-1 and ridge ranking metrics.
func logSurface(t *testing.T, name string, s SurfaceBaseline) {
	t.Helper()
	t.Logf("[%s] rank1: seeds=%d NDCG@%d=%.4f Recall@%d=%.4f", name, s.Rank1.Queries, evalK, s.Rank1.NDCG, evalK, s.Rank1.Recall)
	t.Logf("[%s] ridge: seeds=%d NDCG@%d=%.4f Recall@%d=%.4f", name, s.Ridge.Queries, evalK, s.Ridge.NDCG, evalK, s.Ridge.Recall)
	t.Logf("[%s] ridge − rank1: NDCG %+.4f Recall %+.4f", name, s.Ridge.NDCG-s.Rank1.NDCG, s.Ridge.Recall-s.Rank1.Recall)
}

// assertFixture asserts one fixture's search, explore, and person metrics
// against baseline.
func assertFixture(t *testing.T, fixture string, got, want FixtureBaseline) {
	t.Helper()
	assertPath(t, fixture+"/rank1", got.Rank1, want.Rank1)
	assertPath(t, fixture+"/ridge", got.Ridge, want.Ridge)
	assertSurface(t, fixture+"/explore", got.Explore, want.Explore)
	assertSurface(t, fixture+"/person", got.Person, want.Person)
}

// assertSurface asserts one derived surface's rank-1 and ridge ranking metrics.
func assertSurface(t *testing.T, name string, got, want SurfaceBaseline) {
	t.Helper()
	assertSurfacePath(t, name+"/rank1", got.Rank1, want.Rank1)
	assertSurfacePath(t, name+"/ridge", got.Ridge, want.Ridge)
}

func assertSurfacePath(t *testing.T, name string, got, want SearchMetrics) {
	t.Helper()
	if got.Queries != want.Queries {
		t.Errorf("[%s] fixture set changed: %d entries, baseline has %d — rerun with -update", name, got.Queries, want.Queries)
	}
	assertNoRegression(t, name+" NDCG@8", got.NDCG, want.NDCG)
	assertNoRegression(t, name+" Recall@8", got.Recall, want.Recall)
}

// assertPath asserts one similarity model's metrics against its baseline entry.
// The GCV λ is deliberately not asserted: it is re-derived each run, so its
// effect reaches the assertions through the ridge search metrics.
func assertPath(t *testing.T, name string, got, want PathMetrics) {
	t.Helper()
	if got.Search.Queries != want.Search.Queries {
		t.Errorf("[%s] judgment set changed: %d queries, baseline has %d — rerun with -update", name, got.Search.Queries, want.Search.Queries)
	}
	assertNoRegression(t, name+" NDCG@8", got.Search.NDCG, want.Search.NDCG)
	assertNoRegression(t, name+" Recall@8", got.Search.Recall, want.Search.Recall)

	assertWithinDrift(t, name+" factorWeight", got.FactorModel.FactorWeight, want.FactorModel.FactorWeight)
	assertWithinDrift(t, name+" aggregateR2", got.FactorModel.AggregateR2, want.FactorModel.AggregateR2)
	assertWithinDrift(t, name+" r2.mean", got.FactorModel.R2.Mean, want.FactorModel.R2.Mean)
	assertWithinDrift(t, name+" r2.median", got.FactorModel.R2.Median, want.FactorModel.R2.Median)
	assertWithinDrift(t, name+" r2.p10", got.FactorModel.R2.P10, want.FactorModel.R2.P10)
	assertWithinDrift(t, name+" r2.p90", got.FactorModel.R2.P90, want.FactorModel.R2.P90)
	if got.FactorModel.R2.Count != want.FactorModel.R2.Count {
		t.Errorf("[%s] r2.count = %d, baseline %d — corpus changed, rerun with -update", name, got.FactorModel.R2.Count, want.FactorModel.R2.Count)
	}
}

// logPath prints one similarity model's metrics under -v.
func logPath(t *testing.T, name string, p PathMetrics) {
	t.Helper()
	t.Logf("[%s] search: queries=%d NDCG@%d=%.4f Recall@%d=%.4f",
		name, p.Search.Queries, evalK, p.Search.NDCG, evalK, p.Search.Recall)
	t.Logf("[%s] factor model: factorWeight=%.4f aggregateR2=%.4f", name, p.FactorModel.FactorWeight, p.FactorModel.AggregateR2)
	t.Logf("[%s] per-issue R²: n=%d mean=%.4f median=%.4f p10=%.4f p90=%.4f",
		name, p.FactorModel.R2.Count, p.FactorModel.R2.Mean, p.FactorModel.R2.Median,
		p.FactorModel.R2.P10, p.FactorModel.R2.P90)
}

// assertNoRegression fails when a quality metric drops below the baseline.
// Improvements pass but suggest ratcheting the baseline up.
func assertNoRegression(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got < want-rankingTolerance {
		t.Errorf("%s regressed: got %.4f, baseline %.4f — investigate, or rerun with -update if the trade-off is intentional", name, got, want)
	} else if got > want+rankingTolerance {
		t.Logf("%s improved: got %.4f, baseline %.4f — consider ratcheting the baseline with -update", name, got, want)
	}
}

// assertWithinDrift fails when a descriptive metric moves in either
// direction, so changes to the decomposition math are always acknowledged.
func assertWithinDrift(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > r2Tolerance {
		t.Errorf("%s drifted: got %.4f, baseline %.4f — rerun with -update if this change is intentional", name, got, want)
	}
}

// computeFixtureMetrics runs the full rank-1 and ridge-GCV evaluation over one
// fixture corpus. Both fixtures share the same judgment set (judgments grade
// query↔issue relevance, not vectors), so the real fixture reuses judgments.json
// unchanged.
func computeFixtureMetrics(t *testing.T, corpus Corpus) FixtureBaseline {
	t.Helper()
	judgments, err := LoadJudgments(judgmentsPath, corpus)
	if err != nil {
		t.Fatalf("load judgments: %v", err)
	}

	// All fixture issues share the same age, so the freshness multiplier is
	// uniform and metrics do not depend on when the test runs.
	now := time.Now().UTC()
	storeIssues := corpus.StoreIssues(now)
	storeTags := corpus.StoreTags()
	tagNames := corpus.TagNames()

	// Center against the corpus means and select the ridge penalty by GCV, the
	// same call shape ridgelambda.Cache.compute runs in production.
	issueEmb, tagEmb, _, gcvLambda := ridgeGCVFixture(t, corpus, storeIssues)

	// Rank-1 fallback path: search with no options, the model small or
	// degenerate corpora still fall back to.
	rank1NDCG, rank1Recall := rankingMetrics(t, corpus, judgments, storeIssues, storeTags)
	rank1Decomp := issuemath.ComputeFactorDecomposition(storeIssues, tagNames, issueEmb, tagEmb)
	if !rank1Decomp.Decomposed() {
		t.Fatal("factor decomposition fell back to hardcoded weights; the corpus should always be large enough to decompose")
	}

	// Ridge default path: mirror the API layer injecting WithRidgeSimilarity at
	// the GCV-selected penalty.
	ridgeNDCG, ridgeRecall := rankingMetrics(t, corpus, judgments, storeIssues, storeTags,
		issuemap.WithRidgeSimilarity(gcvLambda))
	ridgeDecomp := issuemath.ComputeRidgeDecomposition(storeIssues, tagNames, issueEmb, tagEmb,
		scoring.RidgeAnchorLambdaScored, gcvLambda)
	if !ridgeDecomp.Decomposed() {
		t.Fatal("ridge decomposition fell back to hardcoded weights; the corpus should always be large enough to decompose")
	}

	// Derived-surface coverage (WP-302): explore (seeded by an issue) and person
	// recommendation (seeded by a profile) inherit the same blend + ridge default
	// but a different query shape, so each is evaluated on both models through its
	// production entry point at the same GCV penalty.
	explore := exploreSurfaceMetrics(t, corpus, storeIssues, storeTags, gcvLambda)
	person := personSurfaceMetrics(t, corpus, storeIssues, storeTags)

	return FixtureBaseline{
		Rank1: PathMetrics{
			Search:      searchMetrics(len(judgments), rank1NDCG, rank1Recall),
			FactorModel: factorMetrics(rank1Decomp.FactorWeight, rank1Decomp.AggregateR2, collectR2(rank1Decomp.AllR2)),
		},
		Ridge: PathMetrics{
			Search:            searchMetrics(len(judgments), ridgeNDCG, ridgeRecall),
			FactorModel:       factorMetrics(ridgeDecomp.FactorWeight, ridgeDecomp.AggregateR2, collectR2(ridgeDecomp.AllR2)),
			GCVLambdaUnscored: round4(gcvLambda),
		},
		Explore: explore,
		Person:  person,
	}
}

// ridgeGCVFixture centers the fixture corpus embeddings against the corpus
// means and selects the unscored ridge penalty by GCV with the scored penalty
// held fixed — the same call shape ridgelambda.Cache.compute runs in
// production. The golden ridge baseline and TestRidgeShadowComparison both
// derive λ through here so they stay on one selection path.
func ridgeGCVFixture(t *testing.T, corpus Corpus, storeIssues []issues.Issue) (issueEmb, tagEmb map[string][]float64, means issuemath.CorpusMeans, lambda float64) {
	t.Helper()
	issueEmb, tagEmb, means = issuemath.CenterEmbeddings(corpus.IssueEmbeddings(), corpus.TagEmbeddings())
	lambda, ok := issuemath.SelectRidgeLambdaGCV(storeIssues, corpus.TagNames(), issueEmb, tagEmb,
		scoring.RidgeAnchorLambdaScored, nil)
	if !ok {
		t.Fatal("GCV λ selection fell back; the fixture corpus should be large enough to select a penalty")
	}
	return issueEmb, tagEmb, means, lambda
}

func searchMetrics(queries int, ndcg, recall float64) SearchMetrics {
	return SearchMetrics{Queries: queries, NDCG: round4(ndcg), Recall: round4(recall)}
}

func factorMetrics(factorWeight, aggregateR2 float64, r2s []float64) FactorModelMetrics {
	d := Summarize(r2s)
	return FactorModelMetrics{
		FactorWeight: round4(factorWeight),
		AggregateR2:  round4(aggregateR2),
		R2: Distribution{
			Count:  d.Count,
			Mean:   round4(d.Mean),
			Median: round4(d.Median),
			P10:    round4(d.P10),
			P90:    round4(d.P90),
		},
	}
}

// rankingMetrics drives the production search entry point for every labeled
// query and returns mean NDCG@K and Recall@K. Extra search options are
// forwarded, which is how the sweep harness injects weight overrides.
func rankingMetrics(
	t *testing.T,
	corpus Corpus,
	judgments []Judgment,
	storeIssues []issues.Issue,
	storeTags []issues.Tag,
	opts ...issuemap.SearchOption,
) (ndcg, recall float64) {
	t.Helper()
	var ndcgSum, recallSum float64
	for _, judgment := range judgments {
		query, ok := corpus.QueryByID(judgment.Query)
		if !ok {
			t.Fatalf("judgment query %q not in corpus", judgment.Query)
		}
		resp := issuemap.SearchFromQueryWithTags(
			storeIssues,
			storeTags,
			query.Raw,
			toTagRelevances(query.Tags),
			query.Embedding,
			evalK,
			opts...,
		)
		ranked := make([]string, len(resp.RelatedIssues))
		for i, ri := range resp.RelatedIssues {
			ranked[i] = ri.ID
		}
		ndcgSum += NDCGAtK(ranked, judgment.Expected, evalK)
		recallSum += RecallAtK(ranked, judgment.Expected, evalK)
	}
	queryCount := float64(len(judgments))
	return ndcgSum / queryCount, recallSum / queryCount
}

func round4(v float64) float64 {
	return math.Round(v*1e4) / 1e4
}
