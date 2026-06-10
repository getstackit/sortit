package matheval

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"sortit/internal/issuemath"
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

// Baseline is the golden metrics file. Regenerate after an intentional math
// change with `go test ./internal/matheval -update`.
type Baseline struct {
	Search      SearchMetrics      `json:"search"`
	FactorModel FactorModelMetrics `json:"factorModel"`
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
// every labeled query, summarizes per-issue R² from the factor decomposition,
// and compares both against the recorded baseline. Run with -v to see the
// metrics; see docs/math-eval.md for interpretation.
func TestMathEval(t *testing.T) {
	got := computeMetrics(t)

	t.Logf("search: queries=%d NDCG@%d=%.4f Recall@%d=%.4f",
		got.Search.Queries, evalK, got.Search.NDCG, evalK, got.Search.Recall)
	t.Logf("factor model: factorWeight=%.4f aggregateR2=%.4f", got.FactorModel.FactorWeight, got.FactorModel.AggregateR2)
	t.Logf("per-issue R²: n=%d mean=%.4f median=%.4f p10=%.4f p90=%.4f",
		got.FactorModel.R2.Count, got.FactorModel.R2.Mean, got.FactorModel.R2.Median,
		got.FactorModel.R2.P10, got.FactorModel.R2.P90)

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

	if got.Search.Queries != want.Search.Queries {
		t.Errorf("judgment set changed: %d queries, baseline has %d — rerun with -update", got.Search.Queries, want.Search.Queries)
	}
	assertNoRegression(t, "NDCG@8", got.Search.NDCG, want.Search.NDCG)
	assertNoRegression(t, "Recall@8", got.Search.Recall, want.Search.Recall)

	assertWithinDrift(t, "factorWeight", got.FactorModel.FactorWeight, want.FactorModel.FactorWeight)
	assertWithinDrift(t, "aggregateR2", got.FactorModel.AggregateR2, want.FactorModel.AggregateR2)
	assertWithinDrift(t, "r2.mean", got.FactorModel.R2.Mean, want.FactorModel.R2.Mean)
	assertWithinDrift(t, "r2.median", got.FactorModel.R2.Median, want.FactorModel.R2.Median)
	assertWithinDrift(t, "r2.p10", got.FactorModel.R2.P10, want.FactorModel.R2.P10)
	assertWithinDrift(t, "r2.p90", got.FactorModel.R2.P90, want.FactorModel.R2.P90)
	if got.FactorModel.R2.Count != want.FactorModel.R2.Count {
		t.Errorf("r2.count = %d, baseline %d — corpus changed, rerun with -update", got.FactorModel.R2.Count, want.FactorModel.R2.Count)
	}
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
		)
		ranked := make([]string, len(resp.RelatedIssues))
		for i, ri := range resp.RelatedIssues {
			ranked[i] = ri.ID
		}
		ndcgSum += NDCGAtK(ranked, judgment.Expected, evalK)
		recallSum += RecallAtK(ranked, judgment.Expected, evalK)
	}

	decomp := issuemath.ComputeFactorDecomposition(storeIssues, corpus.TagNames(), corpus.IssueEmbeddings(), corpus.TagEmbeddings())
	if !decomp.Decomposed() {
		t.Fatal("factor decomposition fell back to hardcoded weights; the corpus should always be large enough to decompose")
	}
	r2s := make([]float64, 0, decomp.DecomposedCount())
	decomp.AllR2(func(_ string, r2 float64) {
		r2s = append(r2s, r2)
	})
	r2Dist := Summarize(r2s)

	queryCount := float64(len(judgments))
	return Baseline{
		Search: SearchMetrics{
			Queries: len(judgments),
			NDCG:    round4(ndcgSum / queryCount),
			Recall:  round4(recallSum / queryCount),
		},
		FactorModel: FactorModelMetrics{
			FactorWeight: round4(decomp.FactorWeight),
			AggregateR2:  round4(decomp.AggregateR2),
			R2: Distribution{
				Count:  r2Dist.Count,
				Mean:   round4(r2Dist.Mean),
				Median: round4(r2Dist.Median),
				P10:    round4(r2Dist.P10),
				P90:    round4(r2Dist.P90),
			},
		},
	}
}

func round4(v float64) float64 {
	return math.Round(v*1e4) / 1e4
}
