package issuemap

import "testing"

func TestComputeClustersIncludesTransitiveNeighbors(t *testing.T) {
	issues := []Issue{
		{ID: "1", Tags: []TagRelevance{{Tag: "bug", Relevance: 1}}},
		{ID: "2", Tags: []TagRelevance{{Tag: "bug", Relevance: 1}}},
		{ID: "3", Tags: []TagRelevance{{Tag: "bug", Relevance: 1}}},
		{ID: "4", Tags: []TagRelevance{{Tag: "feature", Relevance: 1}}},
	}
	positions := map[string]Position{
		"1": {X: 0.10, Y: 0.10},
		"2": {X: 0.21, Y: 0.10},
		"3": {X: 0.32, Y: 0.10},
		"4": {X: 0.90, Y: 0.90},
	}

	clusters := ComputeClusters(issues, positions, 0.12)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}

	cluster := clusters[0]
	if cluster.Label != "Bug" {
		t.Fatalf("expected bug cluster label, got %q", cluster.Label)
	}
	if cluster.CenterX != 0.21 || cluster.CenterY != 0.1 {
		t.Fatalf("unexpected cluster center: %+v", cluster)
	}
	if cluster.Radius != 0.11 {
		t.Fatalf("expected radius 0.11, got %f", cluster.Radius)
	}
	if cluster.Color != tagColors["bug"] {
		t.Fatalf("expected bug cluster color, got %q", cluster.Color)
	}
}
