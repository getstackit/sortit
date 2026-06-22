package issuemath

import (
	"testing"

	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/vectors"
)

// orthogonalAxis returns a unit vector along axis k in dim-dimensional space.
func orthogonalAxis(k, dim int) []float64 {
	v := make([]float64, dim)
	v[k] = 1
	return v
}

// TestSelectRidgeLambdaGCV_AdaptsToConditioning is the load-bearing test for
// the data-driven penalty: GCV must ask for *more* regularization when the
// tag basis is ill-conditioned relative to the embedding dimension (many
// near-collinear tags nearly spanning the space, the overfitting regime) and
// *less* when tags are well-separated and few relative to the dimension. A
// hardcoded constant cannot do this; the whole point of GCV is that it does.
func TestSelectRidgeLambdaGCV_AdaptsToConditioning(t *testing.T) {
	const dim = 8
	grid := []float64{0.01, 0.03, 0.1, 0.3, 1.0, 3.0, 10.0}

	// Well-conditioned: 3 orthogonal tags in 8 dims (K/D small, near-I gram).
	wellTags := []string{"a", "b", "c"}
	wellEmb := map[string][]float64{
		"a": orthogonalAxis(0, dim),
		"b": orthogonalAxis(1, dim),
		"c": orthogonalAxis(2, dim),
	}

	// Ill-conditioned: 6 tags, three near-collinear pairs, jointly spanning
	// most of the space (K/D large). This is the overfitting-prone regime.
	illTags := []string{"a1", "a2", "b1", "b2", "c1", "c2"}
	illEmb := map[string][]float64{
		"a1": unitVec([]float64{1, 0, 0, 0, 0, 0, 0, 0}),
		"a2": unitVec([]float64{0.98, 0.2, 0, 0, 0, 0, 0, 0}),
		"b1": unitVec([]float64{0, 0, 1, 0, 0, 0, 0, 0}),
		"b2": unitVec([]float64{0, 0, 0.98, 0.2, 0, 0, 0, 0}),
		"c1": unitVec([]float64{0, 0, 0, 0, 1, 0, 0, 0}),
		"c2": unitVec([]float64{0, 0, 0, 0, 0.98, 0.2, 0, 0}),
	}

	makeItems := func(tagNames []string, emb map[string][]float64) ([]issues.Issue, map[string][]float64) {
		items := make([]issues.Issue, 0, 8)
		issueEmb := make(map[string][]float64, 8)
		for i := range 8 {
			id := "i" + string(rune('0'+i))
			// Embedding leans on the first tag plus a little spread, so the
			// analyzer scores one tag and the rest stay unscored (the ones
			// whose penalty GCV is choosing).
			vec := make([]float64, dim)
			base := emb[tagNames[i%len(tagNames)]]
			for d := range dim {
				vec[d] = base[d]
			}
			vec[(i+1)%dim] += 0.3 // off-tag spread → residual to fit
			vectors.NormalizeUnit(vec)
			issueEmb[id] = vec
			items = append(items, issues.Issue{
				ID:        id,
				TagScores: []issues.TagRelevance{{Tag: tagNames[i%len(tagNames)], Relevance: 0.8}},
			})
		}
		return items, issueEmb
	}

	wellItems, wellIssueEmb := makeItems(wellTags, wellEmb)
	illItems, illIssueEmb := makeItems(illTags, illEmb)

	wellLambda, ok := SelectRidgeLambdaGCV(wellItems, wellTags, wellIssueEmb, wellEmb, scoring.RidgeAnchorLambdaScored, grid)
	if !ok {
		t.Fatal("GCV selection failed on well-conditioned catalog")
	}
	illLambda, ok := SelectRidgeLambdaGCV(illItems, illTags, illIssueEmb, illEmb, scoring.RidgeAnchorLambdaScored, grid)
	if !ok {
		t.Fatal("GCV selection failed on ill-conditioned catalog")
	}

	t.Logf("GCV λ_unscored: well-conditioned=%.3f ill-conditioned=%.3f", wellLambda, illLambda)
	if illLambda < wellLambda {
		t.Errorf("expected ill-conditioned catalog to want >= regularization: well=%.3f ill=%.3f", wellLambda, illLambda)
	}
}

// TestSelectRidgeLambdaGCV_RejectsDegenerate covers the guards: too few
// issues, and no tags.
func TestSelectRidgeLambdaGCV_RejectsDegenerate(t *testing.T) {
	tagEmb := map[string][]float64{"a": unitVec([]float64{1, 0, 0, 0})}
	tagNames := []string{"a"}
	items := []issues.Issue{{ID: "i0", TagScores: []issues.TagRelevance{{Tag: "a", Relevance: 0.8}}}}
	issueEmb := map[string][]float64{"i0": unitVec([]float64{1, 0, 0, 0})}

	if _, ok := SelectRidgeLambdaGCV(items, tagNames, issueEmb, tagEmb, scoring.RidgeAnchorLambdaScored, nil); ok {
		t.Error("expected GCV to reject a corpus below MinDecompositionIssues")
	}
	if _, ok := SelectRidgeLambdaGCV(nil, nil, nil, nil, scoring.RidgeAnchorLambdaScored, nil); ok {
		t.Error("expected GCV to reject an empty corpus")
	}
}
