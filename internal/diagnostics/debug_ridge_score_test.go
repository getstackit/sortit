package diagnostics

import (
	"context"
	"math"
	"testing"

	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/tags"
)

func TestDebugRidgeScoreAppliesPerTagLambdas(t *testing.T) {
	ctx := context.Background()
	negation := 0.6
	invSqrt2 := 1 / math.Sqrt2
	// Unit vectors and a nil centering cache keep the geometry exact: with
	// orthonormal tag rows the diagonal solve decouples into
	// f_k = ((Te)_k + λ_k·r_k) / (1 + λ_k).
	store := issues.NewInMemoryStore([]issues.Issue{{
		ID:        "issue-ridge",
		Raw:       "ridge",
		Embedding: []float64{invSqrt2, invSqrt2, 0},
		TagScores: []issues.TagRelevance{
			{Tag: "alpha", Relevance: 0.8},
			{Tag: "gamma", Relevance: 0, Negation: &negation},
		},
	}})
	tagStore := &debugTagStore{}
	catalog := tags.NewCatalogService(tagStore, nil, nil)
	if err := catalog.EnsureStoredTags(ctx, []issues.Tag{
		{Name: "alpha", Embedding: []float64{1, 0, 0}},
		{Name: "beta", Embedding: []float64{0, 1, 0}},
		{Name: "gamma", Embedding: []float64{0, 0, 1}},
	}); err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	result, err := DebugRidgeScoreHandler{Store: store, Catalog: catalog}.Handle(ctx, "issue-ridge")
	if err != nil {
		t.Fatalf("handle debug ridge score: %v", err)
	}
	if result.Lambda != scoring.RidgeAnchorLambdaScored {
		t.Fatalf("expected default scored lambda %v, got %v", scoring.RidgeAnchorLambdaScored, result.Lambda)
	}
	if result.LambdaUnscored != scoring.RidgeAnchorLambdaUnscored {
		t.Fatalf("expected unscored lambda %v, got %v", scoring.RidgeAnchorLambdaUnscored, result.LambdaUnscored)
	}

	rows := make(map[string]DebugRidgeTagRow, len(result.Tags))
	for _, row := range result.Tags {
		rows[row.Tag] = row
	}

	alpha, beta, gamma := rows["alpha"], rows["beta"], rows["gamma"]
	if !alpha.Anchored || !gamma.Anchored {
		t.Fatalf("scored and negated tags should be anchored; alpha=%+v gamma=%+v", alpha, gamma)
	}
	if beta.Anchored {
		t.Fatalf("unscored catalog tag should not be anchored; got %+v", beta)
	}
	if gamma.Anchor != -0.6 {
		t.Fatalf("negation-only tag should anchor at -0.6; got %v", gamma.Anchor)
	}

	// Decoupled expectations (round3-ed): alpha (0.7071 + 0.5·0.8)/1.5,
	// beta 0.7071/1.05, gamma (0 + 0.5·(-0.6))/1.5.
	wantAlpha, wantBeta, wantGamma := 0.738, 0.673, -0.2
	if math.Abs(alpha.Ridge-wantAlpha) > 0.001 || math.Abs(beta.Ridge-wantBeta) > 0.001 || math.Abs(gamma.Ridge-wantGamma) > 0.001 {
		t.Fatalf("unexpected ridge scores: alpha=%v beta=%v gamma=%v, want ~%v ~%v ~%v",
			alpha.Ridge, beta.Ridge, gamma.Ridge, wantAlpha, wantBeta, wantGamma)
	}

	// The weakly anchored tag moves much further from its anchor than the
	// strongly anchored one with equal embedding support.
	if absFloat(beta.Delta) <= absFloat(alpha.Delta) {
		t.Fatalf("unanchored tag should move more freely: beta delta %v vs alpha delta %v", beta.Delta, alpha.Delta)
	}

	if result.DriftCosine <= 0 || result.DriftCosine > 1 {
		t.Fatalf("expected drift cosine in (0, 1] for broadly agreeing vectors; got %v", result.DriftCosine)
	}
}

func TestRound3RoundsNegativeValuesAwayFromZero(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0.1234, 0.123},
		{0.1235, 0.124},
		{-0.1234, -0.123},
		{-0.1235, -0.124},
		{-0.0004, 0}, // negative zero compares equal to zero
		{-0.0006, -0.001},
	}
	for _, c := range cases {
		if got := round3(c.in); got != c.want {
			t.Errorf("round3(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
