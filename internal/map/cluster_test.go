package issuemap

import (
	"testing"

	"splat/internal/issues"
)

func TestClusterTopTagUsesHighestAggregateRelevance(t *testing.T) {
	group := []issues.Issue{
		{ID: "1", TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.8}, {Tag: "ui", Relevance: 0.2}}},
		{ID: "2", TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.7}, {Tag: "feature", Relevance: 0.4}}},
		{ID: "3", TagScores: []TagRelevance{{Tag: "ui", Relevance: 0.6}, {Tag: "bug", Relevance: 0.5}}},
	}

	if got := clusterTopTag(group); got != "bug" {
		t.Fatalf("expected top tag bug, got %q", got)
	}
}
