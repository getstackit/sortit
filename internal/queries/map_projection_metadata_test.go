package queries

import (
	"testing"

	"splat/internal/issues"
	issuemap "splat/internal/map"
)

func TestOverlayProjectionIssueMetadataRefreshesMutableFields(t *testing.T) {
	projection := issuemap.MapProjection{
		MapIssues: []issuemap.MapIssue{
			{
				ID:         "issue-1",
				Raw:        "Old raw",
				Status:     issues.StatusOpen,
				AssignedTo: "Casey",
				X:          0.2,
				Y:          0.8,
			},
		},
	}

	updated := overlayProjectionIssueMetadata(projection, []issues.Issue{
		{
			ID:         "issue-1",
			Raw:        "New raw",
			Status:     issues.StatusClosed,
			AssignedTo: "Jordan",
		},
	})

	if updated.MapIssues[0].Raw != "New raw" {
		t.Fatalf("expected raw to be refreshed, got %q", updated.MapIssues[0].Raw)
	}
	if updated.MapIssues[0].Status != issues.StatusClosed {
		t.Fatalf("expected status to be refreshed, got %q", updated.MapIssues[0].Status)
	}
	if updated.MapIssues[0].AssignedTo != "Jordan" {
		t.Fatalf("expected assignedTo to be refreshed, got %q", updated.MapIssues[0].AssignedTo)
	}
	if updated.MapIssues[0].X != projection.MapIssues[0].X || updated.MapIssues[0].Y != projection.MapIssues[0].Y {
		t.Fatalf("expected coordinates to remain unchanged, got %+v", updated.MapIssues[0])
	}
}

func TestOverlayProjectionIssueMetadataEnablesStatusFilteringWithoutRelayout(t *testing.T) {
	projection := issuemap.MapProjection{
		MapIssues: []issuemap.MapIssue{
			{ID: "issue-open", Status: issues.StatusOpen, X: 0.1, Y: 0.1},
			{ID: "issue-closed", Status: issues.StatusOpen, X: 0.9, Y: 0.9},
		},
		VisibleIssueIDs: map[string]struct{}{
			"issue-open":   {},
			"issue-closed": {},
		},
	}

	updated := overlayProjectionIssueMetadata(projection, []issues.Issue{
		{ID: "issue-open", Status: issues.StatusOpen},
		{ID: "issue-closed", Status: issues.StatusClosed},
	})
	filtered := subsetProjectionByStatus(updated, IssueStatusFilterClosed)

	if len(filtered.MapIssues) != 1 {
		t.Fatalf("expected one closed issue after overlay, got %d", len(filtered.MapIssues))
	}
	if filtered.MapIssues[0].ID != "issue-closed" {
		t.Fatalf("expected closed issue to remain visible, got %q", filtered.MapIssues[0].ID)
	}
	if filtered.MapIssues[0].X != 0.9 || filtered.MapIssues[0].Y != 0.9 {
		t.Fatalf("expected coordinates to remain unchanged, got %+v", filtered.MapIssues[0])
	}
}
