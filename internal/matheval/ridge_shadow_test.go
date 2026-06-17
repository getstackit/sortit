package matheval

import (
	"cmp"
	"flag"
	"slices"
	"testing"
	"time"

	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/scoring"
)

var ridgeShadow = flag.Bool("ridge", false, "run the ridge-vs-rank1 shadow comparison (log-only)")

// Findings from this harness (fixture corpus, 2026-06):
//
//   - With the debug-endpoint default λ_unscored=0.05, ridge *regresses*
//     ranking (NDCG 0.867 vs rank-1 0.874) and reports an inflated R² of
//     ~0.90. That R² is overfitting, not honesty: the λ_unscored sweep below
//     shows aggregate R² falling monotonically (0.90→0.75) as the penalty
//     rises, i.e. 16 weakly-penalized tag directions soaking up embedding
//     variance. The drift-diagnostic endpoint *wants* that weak penalty (so
//     missing tags float up); the ranking path does not.
//   - The ranking penalty should not be a hardcoded constant: the optimum
//     depends on the tag catalog's conditioning and the K/D ratio, both of
//     which the system knows at solve time. SelectRidgeLambdaGCV derives it
//     from the data via generalized cross-validation — no labels, no held-out
//     split. On these fixtures GCV picks λ_unscored≈3.0, which lands ridge
//     TAG-SPACE similarity at NDCG ~0.933 / Recall ~0.964 vs rank-1's
//     0.874 / 0.928 — a win on both, reached without peeking at the
//     judgments. R² at that λ falls to ~0.80, an honest middle ground.
//     Tag-space beats reconstruction-space, settling the Phase 3b
//     similarity-shape fork in favor of cos(f_A,f_B).
//   - Caveat: these fixtures generate embeddings AS relevance-weighted tag
//     sums, so they structurally favor the tag-direction story and cannot
//     show ridge's real edge (analyzer-vs-geometry divergence / the drift
//     signal). The win here is therefore a floor, and GCV — not a fixture-fit
//     constant — is what carries forward: it re-derives λ on whatever real
//     K/D/conditioning production has.
//
// TestRidgeShadowComparison measures the full-rank anchored-ridge model
// against the current rank-1 projection on the fixture corpus, before any
// ranking path consumes the ridge scores (math-evolution.md Phase 3a).
//
// It compares the *similarity model in isolation*: candidates are ranked by
// the blended similarity alone, with none of the freshness/velocity/
// authority/specificity modifiers the production search applies. Those
// modifiers are identical across models, so including them would only dilute
// the comparison. Both ridge similarity shapes — tag-space cos(f_A,f_B) and
// reconstruction-space cos(Tᵀf_A,Tᵀf_B) — are reported so the harness, not a
// guess, picks the shape that becomes the Phase 3b default.
//
// Run with: go test ./internal/matheval -run TestRidgeShadowComparison -ridge -v
func TestRidgeShadowComparison(t *testing.T) {
	if !*ridgeShadow {
		t.Skip("ridge shadow comparison is opt-in: rerun with -ridge -v")
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
	tagNames := corpus.TagNames()

	// Mirror the runtime corpus-load boundary: center issue and tag
	// embeddings against the corpus means before decomposing.
	issueEmb, tagEmb, means := issuemath.CenterEmbeddings(corpus.IssueEmbeddings(), corpus.TagEmbeddings())

	// Rank-1 model.
	fdecomp := issuemath.ComputeFactorDecomposition(storeIssues, tagNames, issueEmb, tagEmb)
	tagCov := issuemath.BuildTagCovariance(tagNames, tagEmb)
	if !fdecomp.Decomposed() {
		t.Fatal("rank-1 decomposition fell back; corpus should be large enough")
	}

	// Ridge model at the debug-endpoint default penalty.
	rdecomp := issuemath.ComputeRidgeDecomposition(storeIssues, tagNames, issueEmb, tagEmb,
		scoring.RidgeAnchorLambdaScored, scoring.RidgeAnchorLambdaUnscored)
	if !rdecomp.Decomposed() {
		t.Fatal("ridge decomposition fell back; corpus should be large enough")
	}

	// Ridge model at the GCV-selected unscored penalty — derived from the
	// data, not hardcoded.
	gcvLambda, ok := issuemath.SelectRidgeLambdaGCV(storeIssues, tagNames, issueEmb, tagEmb,
		scoring.RidgeAnchorLambdaScored, nil)
	if !ok {
		t.Fatal("GCV λ selection fell back; corpus should be large enough")
	}
	gcvDecomp := issuemath.ComputeRidgeDecomposition(storeIssues, tagNames, issueEmb, tagEmb,
		scoring.RidgeAnchorLambdaScored, gcvLambda)

	centeredQuery := func(q CorpusQuery) []float64 {
		return issuemath.CenterVector(q.Embedding, means.Issue)
	}

	rank1 := func(q CorpusQuery) []string {
		qv := issuemath.DecomposeEmbedding(centeredQuery(q), toTagRelevances(q.Tags), tagNames, tagEmb, tagCov)
		return rankByScore(storeIssues, func(id string) (float64, bool) {
			cv, ok := fdecomp.DecomposedFor(id)
			if !ok {
				return 0, false
			}
			_, _, blended := issuemath.BlendFromDecomposition(fdecomp, qv, cv)
			return blended, true
		})
	}

	ridge := func(q CorpusQuery, mode issuemath.RidgeSimilarityMode) []string {
		qv := issuemath.DecomposeRidgeEmbedding(centeredQuery(q), toTagRelevances(q.Tags), tagNames, tagEmb,
			scoring.RidgeAnchorLambdaScored, scoring.RidgeAnchorLambdaUnscored)
		return rankByScore(storeIssues, func(id string) (float64, bool) {
			cv, ok := rdecomp.VectorsFor(id)
			if !ok {
				return 0, false
			}
			_, _, blended := issuemath.RidgeBlend(rdecomp, qv, cv, mode)
			return blended, true
		})
	}

	ridgeAt := func(d issuemath.RidgeDecomposition, lambda float64, mode issuemath.RidgeSimilarityMode) func(CorpusQuery) []string {
		return func(q CorpusQuery) []string {
			qv := issuemath.DecomposeRidgeEmbedding(centeredQuery(q), toTagRelevances(q.Tags), tagNames, tagEmb,
				scoring.RidgeAnchorLambdaScored, lambda)
			return rankByScore(storeIssues, func(id string) (float64, bool) {
				cv, ok := d.VectorsFor(id)
				if !ok {
					return 0, false
				}
				_, _, blended := issuemath.RidgeBlend(d, qv, cv, mode)
				return blended, true
			})
		}
	}

	r1NDCG, r1Recall := shadowMetrics(corpus, judgments, rank1)
	rtNDCG, rtRecall := shadowMetrics(corpus, judgments, func(q CorpusQuery) []string { return ridge(q, issuemath.RidgeTagSpace) })
	rrNDCG, rrRecall := shadowMetrics(corpus, judgments, func(q CorpusQuery) []string { return ridge(q, issuemath.RidgeReconSpace) })
	gtNDCG, gtRecall := shadowMetrics(corpus, judgments, ridgeAt(gcvDecomp, gcvLambda, issuemath.RidgeTagSpace))
	grNDCG, grRecall := shadowMetrics(corpus, judgments, ridgeAt(gcvDecomp, gcvLambda, issuemath.RidgeReconSpace))

	t.Logf("%-26s  %8s  %8s  %8s", "model", "NDCG@8", "Recall@8", "w_F")
	t.Logf("%-26s  %8.4f  %8.4f  %8.4f", "rank-1 (current)", r1NDCG, r1Recall, fdecomp.FactorWeight)
	t.Logf("%-26s  %8.4f  %8.4f  %8.4f", "ridge tag-space (λ=0.05)", rtNDCG, rtRecall, rdecomp.FactorWeight)
	t.Logf("%-26s  %8.4f  %8.4f  %8.4f", "ridge recon-space (λ=0.05)", rrNDCG, rrRecall, rdecomp.FactorWeight)
	t.Logf("%-26s  %8.4f  %8.4f  %8.4f", "ridge tag-space (GCV)", gtNDCG, gtRecall, gcvDecomp.FactorWeight)
	t.Logf("%-26s  %8.4f  %8.4f  %8.4f", "ridge recon-space (GCV)", grNDCG, grRecall, gcvDecomp.FactorWeight)
	t.Logf("GCV-selected λ_unscored = %.3f (default was %.3f)", gcvLambda, scoring.RidgeAnchorLambdaUnscored)

	r1Dist := Summarize(collectR2(fdecomp.AllR2))
	defaultDist := Summarize(collectR2(rdecomp.AllR2))
	gcvDist := Summarize(collectR2(gcvDecomp.AllR2))
	t.Logf("R² (rank-1, squared-cosine):     mean=%.4f median=%.4f p10=%.4f p90=%.4f", r1Dist.Mean, r1Dist.Median, r1Dist.P10, r1Dist.P90)
	t.Logf("R² (ridge recon, λ=0.05 overfit): mean=%.4f median=%.4f p10=%.4f p90=%.4f", defaultDist.Mean, defaultDist.Median, defaultDist.P10, defaultDist.P90)
	t.Logf("R² (ridge recon, GCV λ):          mean=%.4f median=%.4f p10=%.4f p90=%.4f", gcvDist.Mean, gcvDist.Median, gcvDist.P10, gcvDist.P90)

	// The GCV ridge is the candidate that would actually ship; tag-space is
	// the winning similarity shape.
	t.Logf("GCV ridge tag-space NDCG@8 vs rank-1: %+.4f (Recall %+.4f)", gtNDCG-r1NDCG, gtRecall-r1Recall)

	// Does the high ridge R² reflect tags explaining the text, or unscored
	// tags overfitting the embedding under a weak penalty? Sweep λ_unscored:
	// if R² collapses as the penalty rises, the high R² was overfitting.
	t.Logf("--- λ_unscored sweep (scored λ fixed at %.2f) ---", scoring.RidgeAnchorLambdaScored)
	t.Logf("%10s  %8s  %10s  %10s", "λ_unscored", "aggR²", "recon NDCG", "tag NDCG")
	for _, lu := range []float64{0.05, 0.25, 1.0, 4.0, 16.0} {
		sweepDecomp := issuemath.ComputeRidgeDecomposition(storeIssues, tagNames, issueEmb, tagEmb,
			scoring.RidgeAnchorLambdaScored, lu)
		reconNDCG, _ := shadowMetrics(corpus, judgments, func(q CorpusQuery) []string {
			qv := issuemath.DecomposeRidgeEmbedding(centeredQuery(q), toTagRelevances(q.Tags), tagNames, tagEmb,
				scoring.RidgeAnchorLambdaScored, lu)
			return rankByScore(storeIssues, func(id string) (float64, bool) {
				cv, ok := sweepDecomp.VectorsFor(id)
				if !ok {
					return 0, false
				}
				_, _, blended := issuemath.RidgeBlend(sweepDecomp, qv, cv, issuemath.RidgeReconSpace)
				return blended, true
			})
		})
		tagNDCG, _ := shadowMetrics(corpus, judgments, func(q CorpusQuery) []string {
			qv := issuemath.DecomposeRidgeEmbedding(centeredQuery(q), toTagRelevances(q.Tags), tagNames, tagEmb,
				scoring.RidgeAnchorLambdaScored, lu)
			return rankByScore(storeIssues, func(id string) (float64, bool) {
				cv, ok := sweepDecomp.VectorsFor(id)
				if !ok {
					return 0, false
				}
				_, _, blended := issuemath.RidgeBlend(sweepDecomp, qv, cv, issuemath.RidgeTagSpace)
				return blended, true
			})
		})
		t.Logf("%10.2f  %8.4f  %10.4f  %10.4f", lu, sweepDecomp.AggregateR2, reconNDCG, tagNDCG)
	}
}

// rankByScore scores every candidate and returns IDs sorted by descending
// score (ties broken by ID for determinism). Candidates the scorer rejects
// are dropped.
func rankByScore(storeIssues []issues.Issue, score func(id string) (float64, bool)) []string {
	type scored struct {
		id    string
		value float64
	}
	ranked := make([]scored, 0, len(storeIssues))
	for _, issue := range storeIssues {
		if v, ok := score(issue.ID); ok {
			ranked = append(ranked, scored{id: issue.ID, value: v})
		}
	}
	slices.SortFunc(ranked, func(a, b scored) int {
		if diff := cmp.Compare(b.value, a.value); diff != 0 {
			return diff
		}
		return cmp.Compare(a.id, b.id)
	})
	ids := make([]string, len(ranked))
	for i, s := range ranked {
		ids[i] = s.id
	}
	return ids
}

func shadowMetrics(corpus Corpus, judgments []Judgment, rank func(CorpusQuery) []string) (ndcg, recall float64) {
	var ndcgSum, recallSum float64
	for _, judgment := range judgments {
		query, ok := corpus.QueryByID(judgment.Query)
		if !ok {
			continue
		}
		ranked := rank(query)
		ndcgSum += NDCGAtK(ranked, judgment.Expected, evalK)
		recallSum += RecallAtK(ranked, judgment.Expected, evalK)
	}
	n := float64(len(judgments))
	return ndcgSum / n, recallSum / n
}

func collectR2(iter func(func(id string, r2 float64))) []float64 {
	var out []float64
	iter(func(_ string, r2 float64) {
		out = append(out, r2)
	})
	return out
}
