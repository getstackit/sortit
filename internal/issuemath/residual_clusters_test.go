package issuemath

import (
	"reflect"
	"sort"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/vectors"
)

// residualTestCorpus builds a decomposition where exp* issues are well explained
// by the "alpha" tag (near-zero residual) and res* issues carry no tag, so their
// residual is their own embedding — placed at distinct angles to res1 so neighbor
// similarities are distinct and the ordering is deterministic.
func residualTestCorpus(t *testing.T) (FactorDecomposition, []string) {
	t.Helper()
	tagEmb := map[string][]float64{"alpha": unitVec([]float64{1, 0, 0, 0})}
	tagNames := []string{"alpha"}

	embeds := map[string][]float64{
		"exp1": unitVec([]float64{1, 0, 0, 0}),
		"exp2": unitVec([]float64{1, 0, 0, 0}),
		"res1": unitVec([]float64{0, 0, 1, 0}),
		"res2": unitVec([]float64{0, 0, 1, 0.2}), // closest to res1
		"res3": unitVec([]float64{0, 0, 1, 0.6}),
		"res4": unitVec([]float64{0, 0, 1, 1.5}), // furthest of the cluster
	}
	items := []issues.Issue{
		{ID: "exp1", TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}}},
		{ID: "exp2", TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}}},
		{ID: "res1"},
		{ID: "res2"},
		{ID: "res3"},
		{ID: "res4"},
	}
	ids := []string{"exp1", "exp2", "res1", "res2", "res3", "res4"}

	decomp := ComputeFactorDecomposition(items, tagNames, embeds, tagEmb)
	if !decomp.Decomposed() {
		t.Fatal("expected a decomposition")
	}
	return decomp, ids
}

func TestResidualNeighbors(t *testing.T) {
	decomp, _ := residualTestCorpus(t)

	neighbors := decomp.ResidualNeighbors("res1", ResidualNeighborSimThreshold)
	if len(neighbors) == 0 {
		t.Fatal("expected residual neighbors for res1")
	}
	// The explained issues share no residual direction, so they never appear.
	for _, n := range neighbors {
		if n.ID == "exp1" || n.ID == "exp2" {
			t.Fatalf("explained issue %q should not be a residual neighbor", n.ID)
		}
		if n.ID == "res1" {
			t.Fatal("an issue must not be its own residual neighbor")
		}
	}
	// Nearest-first: res2 (closest angle) leads res3 leads res4.
	got := make([]string, len(neighbors))
	for i, n := range neighbors {
		got[i] = n.ID
	}
	if !reflect.DeepEqual(got, []string{"res2", "res3", "res4"}) {
		t.Fatalf("expected neighbors ordered res2, res3, res4, got %v", got)
	}
	for i := 1; i < len(neighbors); i++ {
		if neighbors[i-1].Similarity < neighbors[i].Similarity {
			t.Fatalf("neighbors not sorted by descending similarity: %+v", neighbors)
		}
	}
}

func TestResidualNeighborsParityWithPriorAlgorithm(t *testing.T) {
	decomp, ids := residualTestCorpus(t)

	for _, target := range ids {
		got := decomp.ResidualNeighbors(target, ResidualNeighborSimThreshold)

		// Reproduce the prior debug-handler algorithm: scan all issues, keep those
		// with residual cosine strictly greater than the threshold, sort descending.
		residual := decomp.ResidualEmbedding(target)
		want := make([]ResidualNeighbor, 0)
		if len(residual) > 0 {
			for _, id := range ids {
				if id == target {
					continue
				}
				other := decomp.ResidualEmbedding(id)
				if len(other) == 0 {
					continue
				}
				sim := vectors.CosineSimilarity(residual, other)
				if sim > ResidualNeighborSimThreshold {
					want = append(want, ResidualNeighbor{ID: id, Similarity: sim})
				}
			}
			sort.SliceStable(want, func(i, j int) bool { return want[i].Similarity > want[j].Similarity })
		}

		if len(got) != len(want) {
			t.Fatalf("target %q: parity mismatch in count: got %d want %d", target, len(got), len(want))
		}
		for i := range want {
			if got[i].ID != want[i].ID {
				t.Fatalf("target %q: parity mismatch at %d: got %q want %q", target, i, got[i].ID, want[i].ID)
			}
		}
	}
}

func TestTagDataFromIssues(t *testing.T) {
	storeTags := []issues.Tag{
		{Name: "beta", Embedding: []float64{0, 1}},
		{Name: "alpha", Embedding: []float64{1, 0}},
		{Name: "alpha"}, // duplicate name, no embedding — deduped, doesn't clobber
		{Name: "   "},   // blank — skipped
	}
	items := []issues.Issue{
		{ID: "i1", TagScores: []domain.TagRelevance{{Tag: "gamma"}, {Tag: "beta"}}},
		{ID: "i2", TagScores: []domain.TagRelevance{{Tag: ""}}},
	}

	names, embeddings := TagDataFromIssues(items, storeTags)

	// Sorted, deduplicated union of catalog + issue tags.
	if !reflect.DeepEqual(names, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("unexpected tag names: %v", names)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected 2 tag embeddings (alpha, beta), got %d", len(embeddings))
	}
	if !reflect.DeepEqual(embeddings["alpha"], []float64{1, 0}) {
		t.Fatalf("unexpected alpha embedding: %v", embeddings["alpha"])
	}
	if _, ok := embeddings["gamma"]; ok {
		t.Fatal("gamma has no catalog embedding and should be absent from the embedding map")
	}
}
