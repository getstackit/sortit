package issuemap

import (
	"math"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/vectors"
)

// ridgeEquivCorpus returns a two-cluster corpus large enough to decompose, with
// full-corpus means, for the cached-vs-in-place equivalence tests.
func ridgeEquivCorpus() ([]issues.Issue, []issues.Tag) {
	storeIssues := []issues.Issue{
		{ID: "alpha-a", Raw: "alpha issue a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}}, Embedding: unitVec([]float64{0.95, 0.1, 0.1, 0})},
		{ID: "alpha-b", Raw: "alpha issue b", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.85}}, Embedding: unitVec([]float64{0.9, 0.2, 0.1, 0})},
		{ID: "alpha-c", Raw: "alpha issue c", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.8}}, Embedding: unitVec([]float64{0.92, 0.1, 0.2, 0})},
		{ID: "beta-a", Raw: "beta issue a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.9}}, Embedding: unitVec([]float64{0.1, 0.95, 0.1, 0})},
		{ID: "beta-b", Raw: "beta issue b", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.85}}, Embedding: unitVec([]float64{0.2, 0.9, 0.1, 0})},
		{ID: "beta-c", Raw: "beta issue c", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.8}}, Embedding: unitVec([]float64{0.1, 0.92, 0.2, 0})},
	}
	storeTags := []issues.Tag{
		{Name: "alpha", Embedding: unitVec([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unitVec([]float64{0, 1, 0, 0})},
	}
	return storeIssues, storeTags
}

// fullCorpusMeans reproduces the runtime/centering-cache means: issue
// embeddings unit-normalized, tag embeddings as stored.
func fullCorpusMeans(items []issues.Issue, storeTags []issues.Tag) issuemath.CorpusMeans {
	issueEmb := make(map[string][]float64, len(items))
	for _, it := range items {
		if len(it.Embedding) > 0 {
			issueEmb[it.ID] = vectors.NormalizedCopy(it.Embedding)
		}
	}
	tagEmb := make(map[string][]float64, len(storeTags))
	for _, t := range storeTags {
		if len(t.Embedding) > 0 {
			tagEmb[t.Name] = t.Embedding
		}
	}
	return issuemath.ComputeCorpusMeans(issueEmb, tagEmb)
}

// TestRidgeDecompositionEquivalentToInPlaceSearch asserts that scoring search
// via the revision-cached full-corpus decomposition (WithRidgeDecomposition)
// yields identical rankings and scores to the in-place per-request solve
// (WithRidgeSimilarity) when the candidate set is the full corpus.
func TestRidgeDecompositionEquivalentToInPlaceSearch(t *testing.T) {
	storeIssues, storeTags := ridgeEquivCorpus()
	means := fullCorpusMeans(storeIssues, storeTags)
	const lambda = 1.0

	queryTags := []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}}
	queryEmbed := unitVec([]float64{0.95, 0.1, 0.1, 0})

	inPlace := SearchFromQueryWithTags(
		storeIssues, storeTags, "alpha problem", queryTags, queryEmbed, 10,
		WithCorpusMeans(means), WithRidgeSimilarity(lambda),
	)

	cached := ComputeCorpusRidgeDecomposition(storeIssues, storeTags, means,
		scoring.RidgeAnchorLambdaScored, lambda)
	if !cached.Decomposed() {
		t.Fatal("expected the fixture corpus to decompose")
	}
	viaCache := SearchFromQueryWithTags(
		storeIssues, storeTags, "alpha problem", queryTags, queryEmbed, 10,
		WithCorpusMeans(means), WithRidgeDecomposition(cached),
	)

	assertSameRanking(t, inPlace.RelatedIssues, viaCache.RelatedIssues)
}

// TestRidgeDecompositionEquivalentToInPlaceExplore asserts the same equivalence
// for explore across a few seeds.
func TestRidgeDecompositionEquivalentToInPlaceExplore(t *testing.T) {
	storeIssues, storeTags := ridgeEquivCorpus()
	// Explore self-derives means over its candidate set (all open issues here),
	// so the cached bundle must use the same self-derived means.
	means := fullCorpusMeans(storeIssues, storeTags)
	const lambda = 1.0

	cached := ComputeCorpusRidgeDecomposition(storeIssues, storeTags, means,
		scoring.RidgeAnchorLambdaScored, lambda)
	if !cached.Decomposed() {
		t.Fatal("expected the fixture corpus to decompose")
	}

	for _, seed := range []string{"alpha-a", "beta-b", "alpha-c"} {
		inPlace, err := ExploreFromIssuesWithTags(storeIssues, storeTags, seed, 10,
			WithExploreRidgeSimilarity(lambda))
		if err != nil {
			t.Fatalf("explore in-place %s: %v", seed, err)
		}
		viaCache, err := ExploreFromIssuesWithTags(storeIssues, storeTags, seed, 10,
			WithExploreRidgeDecomposition(cached))
		if err != nil {
			t.Fatalf("explore cached %s: %v", seed, err)
		}
		assertSameRanking(t, inPlace.RelatedIssues, viaCache.RelatedIssues)
	}
}

func assertSameRanking(t *testing.T, want, got []RelatedIssue) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("result count differs: in-place %d vs cached %d", len(want), len(got))
	}
	for i := range want {
		w, g := want[i], got[i]
		if w.ID != g.ID {
			t.Fatalf("rank %d: id differs, in-place %q vs cached %q", i, w.ID, g.ID)
		}
		if math.Abs(w.CombinedSimilarity-g.CombinedSimilarity) > 1e-9 {
			t.Fatalf("rank %d (%s): combined differs, %.12f vs %.12f", i, w.ID, w.CombinedSimilarity, g.CombinedSimilarity)
		}
		if math.Abs(w.SemanticSimilarity-g.SemanticSimilarity) > 1e-9 {
			t.Fatalf("rank %d (%s): semantic differs, %.12f vs %.12f", i, w.ID, w.SemanticSimilarity, g.SemanticSimilarity)
		}
		if math.Abs(w.FactorSimilarity-g.FactorSimilarity) > 1e-9 {
			t.Fatalf("rank %d (%s): factor differs, %.12f vs %.12f", i, w.ID, w.FactorSimilarity, g.FactorSimilarity)
		}
	}
}
