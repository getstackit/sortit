package issuemap

import (
	"testing"

	"splat/internal/issues"
)

func TestComputePositionsSingleIssueFallsBackToCenter(t *testing.T) {
	positions, err := ComputePositions([]issues.Issue{
		{
			ID: "1",
			TagScores: []TagRelevance{
				{Tag: "bug", Relevance: 1},
			},
		},
	}, []string{"bug"}, nil)
	if err != nil {
		t.Fatalf("ComputePositions returned error: %v", err)
	}

	pos, ok := positions["1"]
	if !ok {
		t.Fatal("expected position for issue 1")
	}
	if pos.X != 0.5 || pos.Y != 0.5 {
		t.Fatalf("expected centered fallback position, got %+v", pos)
	}
}

func TestComputePositionsSingleTagUsesFallbackLayout(t *testing.T) {
	items := []issues.Issue{
		{ID: "1", TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.9}}},
		{ID: "2", TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.7}}},
		{ID: "3", TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.5}}},
	}

	positions, err := ComputePositions(items, []string{"bug"}, nil)
	if err != nil {
		t.Fatalf("ComputePositions returned error: %v", err)
	}

	if len(positions) != len(items) {
		t.Fatalf("expected %d positions, got %d", len(items), len(positions))
	}

	seen := map[Position]struct{}{}
	for _, issue := range items {
		pos, ok := positions[issue.ID]
		if !ok {
			t.Fatalf("missing position for issue %s", issue.ID)
		}
		if pos.X < 0 || pos.X > 1 || pos.Y < 0 || pos.Y > 1 {
			t.Fatalf("fallback position out of range for issue %s: %+v", issue.ID, pos)
		}
		seen[pos] = struct{}{}
	}

	if len(seen) != len(items) {
		t.Fatalf("expected distinct fallback positions, got %d unique positions", len(seen))
	}
}

func TestNormalizeRobustClipsOutliersBeforeScaling(t *testing.T) {
	vals := []float64{0, 1, 2, 3, 100}

	normalizeRobust(vals, 0.05, 0.95)

	if vals[0] != 0.05 {
		t.Fatalf("expected lower bound to map to 0.05, got %v", vals[0])
	}
	if vals[3] >= 0.8 {
		t.Fatalf("expected inlier values to avoid being crushed by outlier, got %v", vals[3])
	}
	if vals[4] != 0.95 {
		t.Fatalf("expected clipped outlier to map to 0.95, got %v", vals[4])
	}
}
