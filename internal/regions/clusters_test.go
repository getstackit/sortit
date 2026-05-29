package regions

import (
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
)

func TestListClusterRegionsBuildsMetrics(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window, _ := ParseTimeWindow("30d", now)

	items := []issues.Issue{
		{ID: "i1", Status: issues.StatusOpen, CreatedAt: now.Add(-3 * 24 * time.Hour)},
		{ID: "i2", Status: issues.StatusOpen, CreatedAt: now.Add(-2 * 24 * time.Hour)},
		{ID: "i3", Status: issues.StatusClosed, ClosedAt: new(now.Add(-24 * time.Hour)), CreatedAt: now.Add(-20 * 24 * time.Hour)},
		{ID: "j1", Status: issues.StatusOpen, CreatedAt: now.Add(-time.Hour)},
		{ID: "j2", Status: issues.StatusOpen, CreatedAt: now.Add(-time.Hour)},
	}
	clusters := []issuemap.Cluster{
		{ID: "c-aaa", Label: "auth", IssueIDs: []string{"i1", "i2", "i3"}, TopTag: "auth"},
		{ID: "c-bbb", Label: "billing", IssueIDs: []string{"j1", "j2"}, TopTag: "billing"},
	}

	regions := ListClusterRegionsWithMetrics(clusters, items, nil, window, now)
	if len(regions) != 2 {
		t.Fatalf("expected 2 cluster regions, got %d", len(regions))
	}
	// Mass desc — auth cluster has 3 members, billing has 2.
	if regions[0].Region.Key.ID != "c-aaa" {
		t.Fatalf("expected c-aaa first, got %q", regions[0].Region.Key.ID)
	}
	if regions[0].Metrics.Mass != 3 || regions[0].Metrics.MassOpen != 2 || regions[0].Metrics.MassClosed != 1 {
		t.Fatalf("auth metrics unexpected: %+v", regions[0].Metrics)
	}
	if regions[0].Region.Key.Kind != domain.RegionKindCluster {
		t.Fatalf("expected cluster kind, got %q", regions[0].Region.Key.Kind)
	}
	if regions[1].Metrics.Mass != 2 {
		t.Fatalf("billing mass = %d, want 2", regions[1].Metrics.Mass)
	}
}

func TestListClusterRegionsSkipsDeletedMembers(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window, _ := ParseTimeWindow("30d", now)

	items := []issues.Issue{
		// i3 missing from items — perhaps deleted between projection build and now.
		{ID: "i1", Status: issues.StatusOpen, CreatedAt: now},
		{ID: "i2", Status: issues.StatusOpen, CreatedAt: now},
	}
	clusters := []issuemap.Cluster{
		{ID: "c-x", IssueIDs: []string{"i1", "i2", "i3"}, TopTag: "x"},
	}

	regions := ListClusterRegionsWithMetrics(clusters, items, nil, window, now)
	if len(regions) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(regions))
	}
	if regions[0].Metrics.Mass != 2 {
		t.Fatalf("expected mass 2 (deleted issue dropped), got %d", regions[0].Metrics.Mass)
	}
}

func TestListClusterRegionsEmptyInput(t *testing.T) {
	now := time.Now()
	window, _ := ParseTimeWindow("30d", now)
	if got := ListClusterRegionsWithMetrics(nil, nil, nil, window, now); got != nil {
		t.Fatalf("expected nil for empty input; got %+v", got)
	}
}
