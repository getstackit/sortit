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

// Baseline is the golden metrics file. It guards both similarity models search
// can run: the rank-1 fallback (no options) and the ridge tag-space default the
// API layer injects via WithRidgeSimilarity. Regenerate after an intentional
// math change with `go test ./internal/matheval -update`.
type Baseline struct {
	Rank1 PathMetrics `json:"rank1"`
	Ridge PathMetrics `json:"ridge"`
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
// It drives SearchFromQueryWithTags end-to-end over the fixture corpus for
// every labeled query on both similarity models — the rank-1 fallback and the
// ridge tag-space default (WithRidgeSimilarity at the GCV-selected penalty) —
// summarizes per-issue R² for each, and compares both against the recorded
// baseline. Run with -v to see the metrics; see docs/math-eval.md for
// interpretation.
func TestMathEval(t *testing.T) {
	got := computeMetrics(t)

	logPath(t, "rank1", got.Rank1)
	logPath(t, "ridge", got.Ridge)
	t.Logf("ridge GCV λ_unscored = %.4f", got.Ridge.GCVLambdaUnscored)

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

	assertPath(t, "rank1", got.Rank1, want.Rank1)
	assertPath(t, "ridge", got.Ridge, want.Ridge)
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

func computeMetrics(t *testing.T) Baseline {
	t.Helper()
	corpus, err := LoadCorpus(corpusPath)
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
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

	return Baseline{
		Rank1: PathMetrics{
			Search:      searchMetrics(len(judgments), rank1NDCG, rank1Recall),
			FactorModel: factorMetrics(rank1Decomp.FactorWeight, rank1Decomp.AggregateR2, collectR2(rank1Decomp.AllR2)),
		},
		Ridge: PathMetrics{
			Search:            searchMetrics(len(judgments), ridgeNDCG, ridgeRecall),
			FactorModel:       factorMetrics(ridgeDecomp.FactorWeight, ridgeDecomp.AggregateR2, collectR2(ridgeDecomp.AllR2)),
			GCVLambdaUnscored: round4(gcvLambda),
		},
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
