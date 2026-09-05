package matheval

import (
	"flag"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"sortit/internal/issuemath"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
	"sortit/internal/scoring"
	"sortit/internal/vectors"
)

var learnedBasisStudy = flag.Bool("learnedbasis", false, "run the WP-704 learned tag-basis study (log-only report)")

// TestLearnedTagBasisStudy is WP-704's pre-committed offline config matrix.
// It keeps every learned basis inside the test seam: production ranking and
// diagnostic consumers continue to use descriptor embeddings. Run with:
//
//	go test ./internal/matheval -run TestLearnedTagBasisStudy -learnedbasis -v
func TestLearnedTagBasisStudy(t *testing.T) {
	if !*learnedBasisStudy {
		t.Skip("learned-basis study is opt-in: rerun with -learnedbasis -v")
	}
	synthetic, real := loadFixtures(t)
	for _, fixture := range []struct {
		name   string
		corpus Corpus
	}{{"synthetic", synthetic}, {"real", real}} {
		t.Logf("===== WP-704 fixture: %s =====", fixture.name)
		runLearnedBasisStudy(t, fixture.corpus)
	}
}

type basisBuilder func([]issues.Issue, map[string][]float64) map[string][]float64

func runLearnedBasisStudy(t *testing.T, corpus Corpus) {
	t.Helper()
	judgments, err := LoadJudgments(judgmentsPath, corpus)
	if err != nil {
		t.Fatal(err)
	}
	storeIssues := corpus.StoreIssues(time.Now().UTC())
	tagNames := corpus.TagNames()
	uncIssues, descriptors := issuemath.CenterEmbeddingsWith(issuemath.CorpusMeans{}, corpus.IssueEmbeddings(), corpus.TagEmbeddings())
	storeTags := corpus.StoreTags()
	zeroMeans := issuemath.CorpusMeans{Issue: make([]float64, corpus.Dim), Tag: make([]float64, corpus.Dim)}

	type studyArm struct {
		name  string
		build basisBuilder
	}
	gammas := []float64{0.1, 1, 5, 20}
	rows := make([]studyArm, 0, 2+len(gammas))
	rows = append(rows,
		studyArm{"descriptor T", func(_ []issues.Issue, _ map[string][]float64) map[string][]float64 { return cloneBasis(descriptors) }},
		studyArm{"relevance-weighted centroids", func(items []issues.Issue, embeddings map[string][]float64) map[string][]float64 {
			return relevanceCentroidBasis(items, tagNames, embeddings, descriptors)
		}},
	)
	for _, gamma := range gammas {
		rows = append(rows, studyArm{"learned T_L γ₀=" + formatFloat(gamma), func(items []issues.Issue, embeddings map[string][]float64) map[string][]float64 {
			return issuemath.LearnTagBasis(items, tagNames, embeddings, descriptors, gamma)
		}})
	}

	t.Logf("%-32s %8s %8s %8s %8s %8s", "arm", "λ", "NDCG@8", "Recall@8", "R² in", "R² OOF")
	for _, row := range rows {
		basis := row.build(storeIssues, uncIssues)
		lambda, ok := issuemath.SelectRidgeLambdaGCV(storeIssues, tagNames, uncIssues, basis, scoring.RidgeAnchorLambdaScored, nil)
		if !ok {
			t.Fatalf("%s: GCV λ selection failed", row.name)
		}
		decomp := issuemath.ComputeRidgeDecomposition(storeIssues, tagNames, uncIssues, basis, scoring.RidgeAnchorLambdaScored, lambda)
		if !decomp.Decomposed() {
			t.Fatalf("%s: decomposition fell back", row.name)
		}
		bundle := issuemath.CorpusRidgeDecomposition{Decomposition: decomp.WithoutReconstructions(), TagNames: tagNames, TagEmbeddings: basis, Means: zeroMeans, LambdaScored: scoring.RidgeAnchorLambdaScored, LambdaUnscored: lambda}
		ndcg, recall := rankingMetrics(t, corpus, judgments, storeIssues, storeTags, issuemap.WithRidgeDecomposition(bundle))
		oof := crossFittedBasisR2(t, storeIssues, tagNames, uncIssues, row.build)
		t.Logf("%-32s %8.3f %8.4f %8.4f %8.4f %8.4f", row.name, lambda, ndcg, recall, decomp.AggregateR2, oof)
	}
}

func relevanceCentroidBasis(items []issues.Issue, tagNames []string, embeddings, descriptors map[string][]float64) map[string][]float64 {
	basis := cloneBasis(descriptors)
	sums := make(map[string][]float64, len(tagNames))
	weights := make(map[string]float64, len(tagNames))
	for _, tag := range tagNames {
		sums[tag] = make([]float64, len(descriptors[tag]))
	}
	ordered := append([]issues.Issue(nil), items...)
	slices.SortFunc(ordered, func(a, b issues.Issue) int { return strings.Compare(a.ID, b.ID) })
	for _, item := range ordered {
		e := embeddings[item.ID]
		for _, score := range item.TagScores {
			if len(e) == 0 || score.Relevance <= 0 || len(sums[score.Tag]) != len(e) {
				continue
			}
			for i := range e {
				sums[score.Tag][i] += score.Relevance * e[i]
			}
			weights[score.Tag] += score.Relevance
		}
	}
	for _, tag := range tagNames {
		if weights[tag] == 0 || vectors.IsZero(sums[tag]) {
			continue
		}
		for i := range sums[tag] {
			sums[tag][i] /= weights[tag]
		}
		vectors.NormalizeUnit(sums[tag])
		basis[tag] = sums[tag]
	}
	return basis
}

func crossFittedBasisR2(t *testing.T, items []issues.Issue, tagNames []string, embeddings map[string][]float64, build basisBuilder) float64 {
	t.Helper()
	ordered := append([]issues.Issue(nil), items...)
	slices.SortFunc(ordered, func(a, b issues.Issue) int { return strings.Compare(a.ID, b.ID) })
	const folds = 5
	var residual, total float64
	for fold := range folds {
		train := make([]issues.Issue, 0, len(ordered)-len(ordered)/folds)
		trainEmbeddings := make(map[string][]float64, len(ordered))
		for i, item := range ordered {
			if i%folds == fold {
				continue
			}
			train = append(train, item)
			trainEmbeddings[item.ID] = embeddings[item.ID]
		}
		basis := build(train, trainEmbeddings)
		lambda, ok := issuemath.SelectRidgeLambdaGCV(train, tagNames, trainEmbeddings, basis, scoring.RidgeAnchorLambdaScored, nil)
		if !ok {
			t.Fatalf("fold %d: GCV λ selection failed", fold)
		}
		for i, item := range ordered {
			if i%folds != fold {
				continue
			}
			e := embeddings[item.ID]
			rv := issuemath.DecomposeRidgeEmbedding(e, item.TagScores, tagNames, basis, scoring.RidgeAnchorLambdaScored, lambda)
			residual += rv.ResidualNorm * rv.ResidualNorm
			total += dot(e, e)
		}
	}
	if total == 0 {
		return 0
	}
	return 1 - residual/total
}

func cloneBasis(in map[string][]float64) map[string][]float64 {
	out := make(map[string][]float64, len(in))
	for tag, embedding := range in {
		out[tag] = append([]float64(nil), embedding...)
	}
	return out
}

func dot(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func formatFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}
