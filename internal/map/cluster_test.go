package issuemap

import (
	"testing"
)

func TestClusterTopTagUsesHighestAggregateRelevance(t *testing.T) {
	group := []Issue{
		{ID: "1", Tags: []TagRelevance{{Tag: "bug", Relevance: 0.8}, {Tag: "ui", Relevance: 0.2}}},
		{ID: "2", Tags: []TagRelevance{{Tag: "bug", Relevance: 0.7}, {Tag: "feature", Relevance: 0.4}}},
		{ID: "3", Tags: []TagRelevance{{Tag: "ui", Relevance: 0.6}, {Tag: "bug", Relevance: 0.5}}},
	}

	if got := clusterTopTag(group); got != "bug" {
		t.Fatalf("expected top tag bug, got %q", got)
	}
}
