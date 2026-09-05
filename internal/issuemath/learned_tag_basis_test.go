package issuemath

import (
	"reflect"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/scoring"
)

func TestLearnTagBasisDeterministicAndColdTagsExact(t *testing.T) {
	descriptors := map[string][]float64{
		"alpha": {1, 0, 0}, "beta": {0, 1, 0}, "cold": {0, 0, 1},
	}
	items, embeddings := learnedBasisFixture()
	got := LearnTagBasis(items, []string{"alpha", "beta", "cold"}, embeddings, descriptors, 0.5)
	for n := 0; n < 20; n++ {
		if again := LearnTagBasis(items, []string{"alpha", "beta", "cold"}, embeddings, descriptors, 0.5); !reflect.DeepEqual(got, again) {
			t.Fatal("learned basis is not bit-for-bit deterministic")
		}
	}
	if !reflect.DeepEqual(got["cold"], descriptors["cold"]) {
		t.Fatalf("cold tag = %v, want descriptor exactly %v", got["cold"], descriptors["cold"])
	}
}

func TestLearnTagBasisUsageAndDegradation(t *testing.T) {
	descriptors := map[string][]float64{
		"light": {1, 0, 0}, "heavy": {0, 1, 0},
	}
	items := make([]issues.Issue, 0, 12)
	embeddings := make(map[string][]float64)
	for i := range 12 {
		id := string(rune('a' + i))
		items = append(items, issues.Issue{ID: id, TagScores: []domain.TagRelevance{{Tag: "heavy", Relevance: 1}}})
		embeddings[id] = unitVec([]float64{1, 0, 0})
	}
	items[0].TagScores = []domain.TagRelevance{{Tag: "light", Relevance: 1}}
	got := LearnTagBasis(items, []string{"light", "heavy", "missing"}, embeddings, descriptors, 5)
	if dotProduct(got["light"], descriptors["light"]) <= dotProduct(got["heavy"], descriptors["heavy"]) {
		t.Fatalf("lightly-used tag should remain nearer its descriptor: light=%v heavy=%v", got["light"], got["heavy"])
	}
	if !reflect.DeepEqual(got["missing"], []float64{0, 0, 0}) {
		t.Fatalf("missing descriptor row = %v, want zero-row fallback", got["missing"])
	}

	tiny := LearnTagBasis(items[:scoring.MinDecompositionIssues-1], []string{"light", "heavy", "missing"}, embeddings, descriptors, 5)
	if !reflect.DeepEqual(tiny["light"], descriptors["light"]) || !reflect.DeepEqual(tiny["heavy"], descriptors["heavy"]) || !reflect.DeepEqual(tiny["missing"], []float64{0, 0, 0}) {
		t.Fatalf("tiny corpus should return descriptor rows unchanged, with missing rows zeroed: %v", tiny)
	}
}

func learnedBasisFixture() ([]issues.Issue, map[string][]float64) {
	items := make([]issues.Issue, 0, scoring.MinDecompositionIssues)
	embeddings := make(map[string][]float64)
	for i := range scoring.MinDecompositionIssues {
		id := string(rune('a' + i))
		items = append(items, issues.Issue{ID: id, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 1}, {Tag: "beta", Relevance: 0.2}}})
		embeddings[id] = unitVec([]float64{0.8, 0.6, 0})
	}
	return items, embeddings
}
