package ai

import "testing"

func TestNormalizeScoresCanonicalizesAndDeduplicates(t *testing.T) {
	scores := []TagScore{
		{Tag: " Bug ", Relevance: 0.9, Suggested: true, Description: "should be ignored"},
		{Tag: "billing", Relevance: 0.63, Suggested: true, Description: "billing and invoicing workflows"},
		{Tag: "Billing", Relevance: 0.52},
		{Tag: "  ", Relevance: 1},
	}

	normalized := normalizeScores(scores, []Tag{
		{Name: "bug", Description: "software defect"},
		{Name: "feature"},
	})

	if len(normalized) != 2 {
		t.Fatalf("expected 2 normalized tags, got %d", len(normalized))
	}

	if normalized[0].Tag != "bug" {
		t.Fatalf("expected canonical taxonomy tag, got %q", normalized[0].Tag)
	}
	if normalized[0].Suggested {
		t.Fatalf("expected taxonomy tag to not be suggested")
	}
	if normalized[0].Description != "" {
		t.Fatalf("expected taxonomy tag description to be cleared, got %q", normalized[0].Description)
	}

	if normalized[1].Tag != "billing" {
		t.Fatalf("expected merged suggested tag, got %q", normalized[1].Tag)
	}
	if !normalized[1].Suggested {
		t.Fatalf("expected merged tag to remain suggested")
	}
	if normalized[1].Description != "billing and invoicing workflows" {
		t.Fatalf("unexpected suggested tag description %q", normalized[1].Description)
	}
}
