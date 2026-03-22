package issuemap

import (
	"testing"

	"splat/internal/issues"
)

func TestSubsetMapProjectionFiltersWithoutRebuildingPositions(t *testing.T) {
	storeIssues := issues.FixtureIssues()
	projection, err := BuildMapProjection(issues.MapProjectionIssuesFromIssues(storeIssues), nil)
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}

	keep := map[string]struct{}{
		storeIssues[0].ID: {},
		storeIssues[1].ID: {},
	}
	filtered := SubsetMapProjection(projection, keep)

	if len(filtered.MapIssues) != 2 {
		t.Fatalf("expected 2 filtered map issues, got %d", len(filtered.MapIssues))
	}
	filteredPositions := positionsFromMapIssues(filtered.MapIssues)
	projectionPositions := positionsFromMapIssues(projection.MapIssues)
	for id := range keep {
		if filteredPositions[id] != projectionPositions[id] {
			t.Fatalf("expected filtered position for %s to match cached projection", id)
		}
	}
}

func TestSubsetMapProjectionEmptySet(t *testing.T) {
	projection, err := BuildMapProjection(issues.MapProjectionIssuesFromIssues(issues.FixtureIssues()), nil)
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}

	filtered := SubsetMapProjection(projection, map[string]struct{}{})
	if len(filtered.VisibleIssueIDs) != 0 {
		t.Fatalf("expected no visible issues, got %d", len(filtered.VisibleIssueIDs))
	}
	if len(filtered.MapIssues) != 0 {
		t.Fatalf("expected no map issues, got %d", len(filtered.MapIssues))
	}
}

func TestBuildEdgeResponseFromProjectionMatchesRuntimePath(t *testing.T) {
	storeIssues := issues.FixtureIssues()
	projection, err := BuildMapProjection(issues.MapProjectionIssuesFromIssues(storeIssues), nil)
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}

	runtime, err := BuildEdgeResponseFromIssues(storeIssues, nil)
	if err != nil {
		t.Fatalf("build runtime edge response: %v", err)
	}

	fromProjection, err := BuildEdgeResponseFromProjection(projection, nil, minEdgeSimilarity)
	if err != nil {
		t.Fatalf("build projection edge response: %v", err)
	}

	if len(fromProjection.Edges) != len(runtime.Edges) {
		t.Fatalf("expected %d edges from projection, got %d", len(runtime.Edges), len(fromProjection.Edges))
	}
	for i := range runtime.Edges {
		if fromProjection.Edges[i] != runtime.Edges[i] {
			t.Fatalf("unexpected edge at index %d: got %+v want %+v", i, fromProjection.Edges[i], runtime.Edges[i])
		}
	}
}

func TestBuildMapProjectionReturnsUnavailableBelowThreshold(t *testing.T) {
	storeIssues := []issues.MapProjectionIssue{
		{ID: "a", Raw: "a", Tags: []string{"bug"}},
		{ID: "b", Raw: "b", Tags: []string{"bug"}},
		{ID: "c", Raw: "c", Tags: []string{"bug"}},
		{ID: "d", Raw: "d", Tags: []string{"bug"}},
	}

	projection, err := BuildMapProjection(storeIssues, nil)
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}
	if projection.Available {
		t.Fatal("expected projection to be unavailable below minimum issue count")
	}
	if projection.UnavailableReason != mapUnavailableReasonInsufficientIssueCount {
		t.Fatalf("unexpected unavailable reason %q", projection.UnavailableReason)
	}
	if projection.IssueCount != 4 {
		t.Fatalf("expected issue count 4, got %d", projection.IssueCount)
	}
}

func TestBuildMapFromProjectionReturnsUnavailableForSmallSubset(t *testing.T) {
	storeIssues := issues.FixtureIssues()
	projection, err := BuildMapProjection(issues.MapProjectionIssuesFromIssues(storeIssues), nil)
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}

	filtered := SubsetMapProjection(projection, map[string]struct{}{
		storeIssues[0].ID: {},
		storeIssues[1].ID: {},
	})
	resp, err := BuildMapFromProjection(filtered, nil, minEdgeSimilarity)
	if err != nil {
		t.Fatalf("build map from filtered projection: %v", err)
	}
	if resp.Available {
		t.Fatal("expected filtered projection map response to be unavailable")
	}
	if resp.UnavailableReason != mapUnavailableReasonInsufficientIssueCount {
		t.Fatalf("unexpected unavailable reason %q", resp.UnavailableReason)
	}
	if resp.IssueCount != 2 {
		t.Fatalf("expected issue count 2, got %d", resp.IssueCount)
	}
}
