package regions

import (
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

const authTag = "auth"

func TestComputeMassEmptyCorpus(t *testing.T) {
	mass, open, closed := ComputeMass(nil, domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag})
	if mass != 0 || open != 0 || closed != 0 {
		t.Fatalf("expected zeros, got mass=%d open=%d closed=%d", mass, open, closed)
	}
}

func TestComputeMassFloorEdge(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: MembershipFloor - 0.001}}},
		{ID: "b", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: MembershipFloor}}},
		{ID: "c", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: MembershipFloor + 0.01}}},
	}
	mass, open, closed := ComputeMass(items, auth)
	if mass != 2 {
		t.Fatalf("mass = %d, want 2", mass)
	}
	if open != 2 {
		t.Fatalf("open = %d, want 2", open)
	}
	if closed != 0 {
		t.Fatalf("closed = %d, want 0", closed)
	}
}

func TestComputeMassStatusSplit(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.8}}},
		{ID: "b", Status: issues.StatusClosed, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.6}}},
		{ID: "c", Status: issues.StatusClosed, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.5}}},
	}
	mass, open, closed := ComputeMass(items, auth)
	if mass != 3 || open != 1 || closed != 2 {
		t.Fatalf("mass=%d open=%d closed=%d, want 3/1/2", mass, open, closed)
	}
}

func TestComputeMassExcludesNegationOnly(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0, Negation: new(0.6)}}},
	}
	mass, _, _ := ComputeMass(items, auth)
	if mass != 0 {
		t.Fatalf("mass = %d, want 0 (negation-only excluded)", mass)
	}
}

func TestComputeAgeBucketsBoundaries(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}
	mkIssue := func(id string, ageDays float64) issues.Issue {
		return issues.Issue{
			ID:        id,
			Status:    issues.StatusOpen,
			CreatedAt: now.Add(-time.Duration(ageDays * float64(24*time.Hour))),
			TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.7}},
		}
	}
	items := []issues.Issue{
		mkIssue("just-now", 0),
		mkIssue("under-1w", 6.9),
		mkIssue("exactly-1w", 7),
		mkIssue("inside-1-4w", 14),
		mkIssue("exactly-4w", 28),
		mkIssue("inside-1-3m", 60),
		mkIssue("exactly-3m", 90),
		mkIssue("over-3m", 365),
	}
	buckets := ComputeAgeBuckets(items, auth, now)
	if len(buckets) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(buckets))
	}
	want := map[string]int{
		AgeBucketUnderOneWeek:     2, // just-now, under-1w
		AgeBucketOneToFourWeeks:   2, // exactly-1w, inside-1-4w
		AgeBucketOneToThreeMonths: 2, // exactly-4w, inside-1-3m
		AgeBucketThreeMonthsPlus:  2, // exactly-3m, over-3m
	}
	for _, b := range buckets {
		if got := b.Count; got != want[b.Label] {
			t.Errorf("bucket %q = %d, want %d", b.Label, got, want[b.Label])
		}
	}
}

func TestComputeAgeBucketsExcludesClosed(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}
	items := []issues.Issue{
		{
			ID:        "a",
			Status:    issues.StatusClosed,
			CreatedAt: now.Add(-time.Hour),
			TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.9}},
		},
	}
	buckets := ComputeAgeBuckets(items, auth, now)
	for _, b := range buckets {
		if b.Count != 0 {
			t.Fatalf("closed issue should be excluded; bucket %q = %d", b.Label, b.Count)
		}
	}
}

func TestComputeMetricsZeroMassReturnsFalse(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window := domain.TimeWindow{Label: "30d", End: now}
	_, ok := ComputeMetrics(nil, domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}, window, now)
	if ok {
		t.Fatal("expected ok=false for empty corpus")
	}
}

func TestListRegionsWithMetricsSortedByMass(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window := domain.TimeWindow{Label: "30d", End: now}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.9}}},
		{ID: "b", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.5}}},
		{ID: "c", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: "billing", Relevance: 0.7}}},
	}
	regions := ListRegionsWithMetrics(items, window, now)
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(regions))
	}
	if regions[0].Region.Key.ID != authTag {
		t.Fatalf("expected auth first (higher mass); got %q", regions[0].Region.Key.ID)
	}
	if regions[0].Metrics.Mass != 2 || regions[1].Metrics.Mass != 1 {
		t.Fatalf("expected masses 2 and 1, got %d and %d", regions[0].Metrics.Mass, regions[1].Metrics.Mass)
	}
}

func TestListRegionsWithMetricsIgnoresBelowFloor(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window := domain.TimeWindow{Label: "30d", End: now}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{
			{Tag: authTag, Relevance: 0.1},
			{Tag: "billing", Relevance: 0.6},
		}},
	}
	regions := ListRegionsWithMetrics(items, window, now)
	if len(regions) != 1 {
		t.Fatalf("expected 1 region, got %d", len(regions))
	}
	if regions[0].Region.Key.ID != "billing" {
		t.Fatalf("expected billing only; got %q", regions[0].Region.Key.ID)
	}
}
