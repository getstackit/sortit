package issuemap

import "testing"

func TestComputePositionsSingleIssueFallsBackToCenter(t *testing.T) {
	positions, err := ComputePositions([]Issue{
		{
			ID: "1",
			Tags: []TagRelevance{
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
	issues := []Issue{
		{ID: "1", Tags: []TagRelevance{{Tag: "bug", Relevance: 0.9}}},
		{ID: "2", Tags: []TagRelevance{{Tag: "bug", Relevance: 0.7}}},
		{ID: "3", Tags: []TagRelevance{{Tag: "bug", Relevance: 0.5}}},
	}

	positions, err := ComputePositions(issues, []string{"bug"}, nil)
	if err != nil {
		t.Fatalf("ComputePositions returned error: %v", err)
	}

	if len(positions) != len(issues) {
		t.Fatalf("expected %d positions, got %d", len(issues), len(positions))
	}

	seen := map[Position]struct{}{}
	for _, issue := range issues {
		pos, ok := positions[issue.ID]
		if !ok {
			t.Fatalf("missing position for issue %s", issue.ID)
		}
		if pos.X < 0 || pos.X > 1 || pos.Y < 0 || pos.Y > 1 {
			t.Fatalf("fallback position out of range for issue %s: %+v", issue.ID, pos)
		}
		seen[pos] = struct{}{}
	}

	if len(seen) != len(issues) {
		t.Fatalf("expected distinct fallback positions, got %d unique positions", len(seen))
	}
}
