package curation

import (
	"context"
	"testing"
	"time"

	"sortit/internal/issues"
	issuemap "sortit/internal/map"
	"sortit/internal/mapview"
)

type fakeExplorer struct {
	related map[string][]issuemap.RelatedIssue
}

func (f fakeExplorer) Handle(_ context.Context, in mapview.ExploreIssue) (issuemap.ExploreResponse, error) {
	return issuemap.ExploreResponse{RelatedIssues: f.related[in.ID]}, nil
}

func TestDetectDuplicatesClustersSimilarOpenIssues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "a", Raw: "Safari crashes exporting PDF", Status: issues.StatusOpen, Tags: []string{"safari", "crash"}, CreatedAt: base},
		{ID: "b", Raw: "PDF export crash on Safari", Status: issues.StatusOpen, Tags: []string{"safari", "crash"}, CreatedAt: base.AddDate(0, 0, 1)},
		{ID: "c", Raw: "Safari export broken", Status: issues.StatusOpen, Tags: []string{"safari"}, CreatedAt: base.AddDate(0, 0, 2)},
		{ID: "d", Raw: "Add CSV export", Status: issues.StatusOpen, Tags: []string{"export"}, CreatedAt: base.AddDate(0, 0, 3)},
	})

	explorer := fakeExplorer{related: map[string][]issuemap.RelatedIssue{
		"a": {
			{ID: "b", Status: issues.StatusOpen, CombinedSimilarity: 0.90},
			{ID: "c", Status: issues.StatusOpen, CombinedSimilarity: 0.88},
			{ID: "d", Status: issues.StatusOpen, CombinedSimilarity: 0.40},
		},
	}}
	det := NewDetector(store, store, explorer, nil, nil, nil)

	clusters, err := det.DetectDuplicates(ctx, DuplicateParams{MinSimilarity: 0.85})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d: %+v", len(clusters), clusters)
	}
	c := clusters[0]
	if len(c.IssueIDs) != 3 || c.IssueIDs[0] != "a" || c.IssueIDs[1] != "b" || c.IssueIDs[2] != "c" {
		t.Fatalf("expected cluster {a,b,c}, got %v", c.IssueIDs)
	}
	if c.SuggestedCanonical != "a" {
		t.Fatalf("expected oldest issue a as canonical, got %q", c.SuggestedCanonical)
	}
	if len(c.SharedTags) != 1 || c.SharedTags[0] != "safari" {
		t.Fatalf("expected shared tag [safari], got %v", c.SharedTags)
	}
	if c.Similarity < 0.85 {
		t.Fatalf("expected cluster similarity >= 0.85, got %v", c.Similarity)
	}
}

func TestDetectStaleIssuesFlagsInactiveOpenIssues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Now().UTC()

	store := issues.NewInMemoryStore([]issues.Issue{
		{ID: "old", Raw: "Old idea nobody touched", Status: issues.StatusOpen, CreatedAt: now.AddDate(0, 0, -200)},
		{ID: "fresh", Raw: "Just filed this", Status: issues.StatusOpen, CreatedAt: now.AddDate(0, 0, -2)},
		{ID: "closed-old", Raw: "Done long ago", Status: issues.StatusClosed, CreatedAt: now.AddDate(0, 0, -300)},
	})

	det := NewDetector(store, store, fakeExplorer{}, nil, nil, nil)
	// High MaxVelocity isolates the days-inactive filter under test.
	stale, err := det.DetectStaleIssues(ctx, StaleParams{MinDaysInactive: 90, MaxVelocity: 1000})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale issue, got %d: %+v", len(stale), stale)
	}
	if stale[0].IssueID != "old" {
		t.Fatalf("expected only 'old' flagged, got %q", stale[0].IssueID)
	}
	if stale[0].DaysInactive < 90 {
		t.Fatalf("expected days inactive >= 90, got %v", stale[0].DaysInactive)
	}
}
