package issuemap

import (
	"math/rand"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

func TestComputeFactorClustersIsDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec
	items := make([]issues.Issue, 0, 20)
	positions := make(map[string]Position, 20)
	// Two visually distinct clusters in PCA space.
	for i := range 10 {
		id := "a-" + string(rune('0'+i))
		items = append(items, issues.Issue{ID: id})
		positions[id] = Position{X: rng.NormFloat64()*0.1 + 1, Y: rng.NormFloat64() * 0.1}
	}
	for i := range 10 {
		id := "b-" + string(rune('0'+i))
		items = append(items, issues.Issue{ID: id})
		positions[id] = Position{X: rng.NormFloat64()*0.1 - 1, Y: rng.NormFloat64() * 0.1}
	}

	first := ComputeFactorClusters(items, positions)
	second := ComputeFactorClusters(items, positions)

	if len(first) != len(second) {
		t.Fatalf("cluster count differs across runs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("cluster ID differs at index %d: %q vs %q", i, first[i].ID, second[i].ID)
		}
		if len(first[i].IssueIDs) != len(second[i].IssueIDs) {
			t.Fatalf("cluster %d member count differs across runs", i)
		}
	}
}

func TestClusterIDStableAcrossPermutation(t *testing.T) {
	a := clusterID([]string{"x", "y", "z"})
	b := clusterID([]string{"z", "x", "y"})
	c := clusterID([]string{"y", "z", "x"})
	if a != b || b != c {
		t.Fatalf("clusterID should be order-independent; got %q %q %q", a, b, c)
	}
}

func TestClusterIDChangesWithMembership(t *testing.T) {
	a := clusterID([]string{"x", "y", "z"})
	b := clusterID([]string{"x", "y", "z", "w"})
	if a == b {
		t.Fatalf("clusterID should differ when membership differs")
	}
}

func TestClusterTopTagUsesHighestAggregateRelevance(t *testing.T) {
	group := []issues.Issue{
		{ID: "1", TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.8}, {Tag: "ui", Relevance: 0.2}}},
		{ID: "2", TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.7}, {Tag: "feature", Relevance: 0.4}}},
		{ID: "3", TagScores: []domain.TagRelevance{{Tag: "ui", Relevance: 0.6}, {Tag: "bug", Relevance: 0.5}}},
	}

	if got := clusterTopTag(group); got != "bug" {
		t.Fatalf("expected top tag bug, got %q", got)
	}
}
