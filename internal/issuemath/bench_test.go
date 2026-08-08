package issuemath

import (
	"math"
	"strconv"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/vectors"
)

// benchCorpus builds a deterministic synthetic corpus sized to exercise the
// decomposition and GCV hot paths: n issues over k tags in a d-dimensional
// embedding space, each issue scoring a handful of tags.
func benchCorpus(n, k, d int) (items []issues.Issue, tagNames []string, issueEmb, tagEmb map[string][]float64) {
	tagNames = make([]string, k)
	tagEmb = make(map[string][]float64, k)
	for i := range k {
		name := "tag" + strconv.Itoa(i)
		tagNames[i] = name
		v := make([]float64, d)
		x := float64(i)*0.7548 + 0.13
		for j := range v {
			x = math.Mod(x*1.31+float64(j)*0.0009, 2)
			v[j] = x - 1
		}
		vectors.NormalizeUnit(v)
		tagEmb[name] = v
	}

	items = make([]issues.Issue, n)
	issueEmb = make(map[string][]float64, n)
	for i := range n {
		id := "issue" + strconv.Itoa(i)
		v := make([]float64, d)
		x := float64(i)*0.3819 + 0.07
		for j := range v {
			x = math.Mod(x*1.27+float64(j)*0.0011, 2)
			v[j] = x - 1
		}
		vectors.NormalizeUnit(v)
		issueEmb[id] = v

		// Score 3 tags per issue, spread across the catalog.
		scores := make([]domain.TagRelevance, 0, 3)
		for s := range 3 {
			tag := tagNames[(i*3+s)%k]
			scores = append(scores, domain.TagRelevance{Tag: tag, Relevance: 0.5 + 0.1*float64(s)})
		}
		items[i] = issues.Issue{ID: id, TagScores: scores}
	}
	return items, tagNames, issueEmb, tagEmb
}

func BenchmarkComputeFactorDecomposition(b *testing.B) {
	items, tagNames, issueEmb, tagEmb := benchCorpus(300, 40, 64)
	b.ReportAllocs()
	for b.Loop() {
		ComputeFactorDecomposition(items, tagNames, issueEmb, tagEmb)
	}
}

func BenchmarkComputeRidgeDecomposition(b *testing.B) {
	items, tagNames, issueEmb, tagEmb := benchCorpus(300, 40, 64)
	b.ReportAllocs()
	for b.Loop() {
		ComputeRidgeDecomposition(items, tagNames, issueEmb, tagEmb,
			scoring.RidgeAnchorLambdaScored, 0.1)
	}
}

func BenchmarkSelectRidgeLambdaGCV(b *testing.B) {
	items, tagNames, issueEmb, tagEmb := benchCorpus(300, 40, 64)
	b.ReportAllocs()
	for b.Loop() {
		SelectRidgeLambdaGCV(items, tagNames, issueEmb, tagEmb,
			scoring.RidgeAnchorLambdaScored, nil)
	}
}

// gcvShapes are the realistic GCV λ-recompute shapes for WP-105. Samples are
// held at ~200 (enough to extrapolate linearly to the 2000-issue GCV bound;
// the per-sample work is identical, so full-recompute ≈ ns/op × (2000/200)).
var gcvShapes = []struct {
	name          string
	samples, k, d int
}{
	{"production_K200_D1536_S200", 200, 200, 1536},
	{"small_K64_D256_S200", 200, 64, 256},
}

// BenchmarkSelectRidgeLambdaGCVShapes measures a full λ-grid recompute (all 7
// grid points) at production and small shapes. Divide ns/op by 7 for one grid
// point; multiply by 10 (2000/200) to extrapolate to the GCV sample bound.
func BenchmarkSelectRidgeLambdaGCVShapes(b *testing.B) {
	for _, sh := range gcvShapes {
		items, tagNames, issueEmb, tagEmb := benchCorpus(sh.samples, sh.k, sh.d)
		b.Run(sh.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				SelectRidgeLambdaGCV(items, tagNames, issueEmb, tagEmb,
					scoring.RidgeAnchorLambdaScored, nil)
			}
		})
	}
}
