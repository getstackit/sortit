package matheval

import (
	"flag"
	"testing"
	"time"

	"sortit/internal/issuemath"
	issuemap "sortit/internal/map"
)

var sweep = flag.Bool("sweep", false, "run hyperparameter sweep reports (slow, log-only)")

// TestSweepFactorWeight reports ranking quality as a function of the factor
// share w_F of the similarity blend. The production blend identifies w_F
// with the decomposition's aggregate R² (whitepaper §2.4) — a
// variance-explained quantity, not a ranking-utility one — so this sweep is
// the evidence for (or against) that identification: it shows where the
// data-driven weight sits relative to the NDCG@8-optimal region.
//
// It is a report, not a gate: it never fails on metric values. Run with:
//
//	go test ./internal/matheval -run TestSweepFactorWeight -sweep -v
func TestSweepFactorWeight(t *testing.T) {
	if !*sweep {
		t.Skip("sweep reports are opt-in: rerun with -sweep -v")
	}

	corpus, err := LoadCorpus(corpusPath)
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	judgments, err := LoadJudgments(judgmentsPath, corpus)
	if err != nil {
		t.Fatalf("load judgments: %v", err)
	}
	now := time.Now().UTC()
	storeIssues := corpus.StoreIssues(now)
	storeTags := corpus.StoreTags()

	// The data-driven reference point the sweep is judging.
	issueEmbeddings, tagEmbeddings, _ := issuemath.CenterEmbeddings(corpus.IssueEmbeddings(), corpus.TagEmbeddings())
	decomp := issuemath.ComputeFactorDecomposition(storeIssues, corpus.TagNames(), issueEmbeddings, tagEmbeddings)
	dataNDCG, dataRecall := rankingMetrics(t, corpus, judgments, storeIssues, storeTags)

	t.Logf("%6s  %8s  %8s", "w_F", "NDCG@8", "Recall@8")
	bestW, bestNDCG := 0.0, -1.0
	for w := 0.0; w <= 1.0001; w += 0.05 {
		ndcg, recall := rankingMetrics(t, corpus, judgments, storeIssues, storeTags,
			issuemap.WithFactorWeightOverride(w))
		if ndcg > bestNDCG {
			bestW, bestNDCG = w, ndcg
		}
		t.Logf("%6.2f  %8.4f  %8.4f", w, ndcg, recall)
	}
	t.Logf("data-driven w_F=%.4f (aggregate R²): NDCG@8=%.4f Recall@8=%.4f", decomp.FactorWeight, dataNDCG, dataRecall)
	t.Logf("sweep-optimal w_F=%.2f: NDCG@8=%.4f (delta vs data-driven: %+.4f)", bestW, bestNDCG, bestNDCG-dataNDCG)
}
