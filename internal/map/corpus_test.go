package issuemap

import (
	"testing"

	"splat/internal/issues"
)

func TestSubsetMapProjectionFiltersWithoutRebuildingPositions(t *testing.T) {
	storeIssues := issues.FixtureIssues()
	projection, err := BuildMapProjection(storeIssues, nil)
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
	for id := range keep {
		if filtered.Positions[id] != projection.Positions[id] {
			t.Fatalf("expected filtered position for %s to match cached projection", id)
		}
	}
}

func TestSubsetMapProjectionEmptySet(t *testing.T) {
	projection, err := BuildMapProjection(issues.FixtureIssues(), nil)
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}

	filtered := SubsetMapProjection(projection, map[string]struct{}{})
	if len(filtered.VisibleIssueIDs) != 0 {
		t.Fatalf("expected no visible issues, got %d", len(filtered.VisibleIssueIDs))
	}
	if len(filtered.Positions) != 0 {
		t.Fatalf("expected no positions, got %d", len(filtered.Positions))
	}
}
