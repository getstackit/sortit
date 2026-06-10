package search

import (
	"context"
	"testing"

	"sortit/internal/issues"
	"sortit/internal/tagcooccurrence"
)

type stubIssueLister struct {
	items []issues.Issue
}

func (s stubIssueLister) List(context.Context) ([]issues.Issue, error) {
	return s.items, nil
}

func issuesWithTags(n int, tags ...string) []issues.Issue {
	out := make([]issues.Issue, 0, n)
	for range n {
		scores := make([]issues.TagRelevance, 0, len(tags))
		for _, tag := range tags {
			scores = append(scores, issues.TagRelevance{Tag: tag, Relevance: 0.9})
		}
		out = append(out, issues.Issue{TagScores: scores})
	}
	return out
}

func TestRegionOptsSmallCorpusEmitsNoAntiCorrelators(t *testing.T) {
	// bug and feature never co-occur, but with only 4 issues the
	// co-occurrence guards zero out every implicit-negative, so the
	// penalty side of region search must be a no-op: only the region
	// target option is returned.
	items := make([]issues.Issue, 0, 4)
	items = append(items, issuesWithTags(3, "bug")...)
	items = append(items, issuesWithTags(1, "feature")...)

	h := SearchIssuesHandler{
		Cooccurrence: &tagcooccurrence.Cache{Store: stubIssueLister{items: items}},
	}
	storeTags := []issues.Tag{{Name: "bug"}, {Name: "feature"}}

	opts, err := h.regionOpts(context.Background(), "bug", storeTags)
	if err != nil {
		t.Fatalf("regionOpts: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("expected only the region-target option on a small corpus, got %d options", len(opts))
	}
}

func TestRegionOptsLargeCorpusEmitsAntiCorrelators(t *testing.T) {
	// With a corpus large enough to clear the statistical floors, the same
	// query gains a second option carrying the anti-correlator penalties.
	items := make([]issues.Issue, 0, 100)
	items = append(items, issuesWithTags(40, "bug")...)
	items = append(items, issuesWithTags(30, "feature")...)
	items = append(items, issuesWithTags(30, "misc")...)

	h := SearchIssuesHandler{
		Cooccurrence: &tagcooccurrence.Cache{Store: stubIssueLister{items: items}},
	}
	storeTags := []issues.Tag{{Name: "bug"}, {Name: "feature"}}

	opts, err := h.regionOpts(context.Background(), "bug", storeTags)
	if err != nil {
		t.Fatalf("regionOpts: %v", err)
	}
	if len(opts) != 2 {
		t.Fatalf("expected region-target plus anti-correlator options on a large corpus, got %d options", len(opts))
	}
}
