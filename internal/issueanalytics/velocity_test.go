package issueanalytics

import (
	"testing"
	"time"

	"splat/internal/issues"
)

func TestComputeIssueVelocityIgnoresNonMeaningfulOrOldEvents(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	velocity, count := ComputeIssueVelocity([]issues.IssuePost{
		{Sequence: 1, Kind: "report", CreatedAt: now.Add(-24 * time.Hour)},
		{Sequence: 2, Kind: "closed", CreatedAt: now.Add(-24 * time.Hour)},
		{Sequence: 3, Kind: "refinement", CreatedAt: now.Add(-45 * 24 * time.Hour)},
	}, nil, now)

	if count != 0 {
		t.Fatalf("expected zero recent activity count, got %d", count)
	}
	if velocity != 0 {
		t.Fatalf("expected zero velocity, got %v", velocity)
	}
}

func TestComputeIssueVelocityPrefersRecentMixedActivity(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	singleRecent, singleCount := ComputeIssueVelocity([]issues.IssuePost{
		{Sequence: 2, Kind: "refinement", CreatedAt: now.Add(-24 * time.Hour)},
	}, nil, now)
	mixedRecent, mixedCount := ComputeIssueVelocity([]issues.IssuePost{
		{Sequence: 2, Kind: "refinement", CreatedAt: now.Add(-24 * time.Hour)},
		{Sequence: 3, Kind: "progress", CreatedAt: now.Add(-48 * time.Hour)},
	}, []issues.IssueLink{
		{CreatedAt: now.Add(-12 * time.Hour)},
	}, now)

	if singleCount != 1 {
		t.Fatalf("expected one recent activity event, got %d", singleCount)
	}
	if mixedCount != 3 {
		t.Fatalf("expected three recent activity events, got %d", mixedCount)
	}
	if mixedRecent <= singleRecent {
		t.Fatalf("expected mixed activity velocity %v > single event velocity %v", mixedRecent, singleRecent)
	}
}

func TestAttachIssueVelocityPopulatesMetrics(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	metrics := AttachIssueVelocity([]issues.IssuePost{
		{Sequence: 2, Kind: "progress", CreatedAt: now.Add(-6 * time.Hour)},
	}, nil, nil, now)

	if metrics == nil || metrics.Velocity == nil {
		t.Fatalf("expected velocity metrics, got %#v", metrics)
	}
	if *metrics.Velocity <= 0 {
		t.Fatalf("expected positive velocity, got %#v", metrics)
	}
	if metrics.RecentActivityCount != 1 {
		t.Fatalf("expected recent activity count 1, got %#v", metrics)
	}
}
