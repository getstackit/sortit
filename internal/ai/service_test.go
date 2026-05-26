package ai

import (
	"context"
	"testing"
)

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

func TestNormalizeScoresRoundsRelevanceToTwoDecimals(t *testing.T) {
	scores := []TagScore{
		{Tag: "billing", Relevance: 0.666},
		{Tag: "crash", Relevance: 1.234},
		{Tag: "ux", Relevance: -0.014},
	}

	normalized := normalizeScores(scores, nil)
	if len(normalized) != 3 {
		t.Fatalf("expected 3 normalized tags, got %d", len(normalized))
	}

	values := map[string]float64{}
	for _, score := range normalized {
		values[score.Tag] = score.Relevance
	}

	if values["billing"] != 0.67 {
		t.Fatalf("expected billing relevance 0.67, got %v", values["billing"])
	}
	if values["crash"] != 1 {
		t.Fatalf("expected crash relevance 1, got %v", values["crash"])
	}
	if values["ux"] != 0 {
		t.Fatalf("expected ux relevance 0, got %v", values["ux"])
	}
}

func TestNormalizeNegatedKeepsOnlyEvidencedTaxonomyTags(t *testing.T) {
	taxonomy := []Tag{
		{Name: "regression"},
		{Name: "safari"},
	}

	negated := []NegatedTag{
		{Tag: " Regression ", Confidence: 0.9, Evidence: []string{"this is not a regression"}},
		{Tag: "safari", Confidence: 0.4, Evidence: []string{}},                        // dropped: no evidence
		{Tag: "feature", Confidence: 0.6, Evidence: []string{"not a feature"}},        // dropped: not in taxonomy
		{Tag: "regression", Confidence: 0.5, Evidence: []string{"never worked"}},      // dedup: keeps higher confidence
		{Tag: "", Confidence: 0.7, Evidence: []string{"never worked"}},                // dropped: empty tag
		{Tag: "safari", Confidence: 1.2, Evidence: []string{"all browsers affected"}}, // cap at 0.7
	}

	out := normalizeNegated(negated, taxonomy)
	if len(out) != 2 {
		t.Fatalf("expected 2 normalized negations, got %d: %+v", len(out), out)
	}

	byTag := make(map[string]NegatedTag, len(out))
	for _, n := range out {
		byTag[n.Tag] = n
	}

	reg, ok := byTag["regression"]
	if !ok {
		t.Fatalf("expected regression to be present")
	}
	// 0.9 is capped at the 0.7 negation cap; merge keeps the higher of the
	// two regression entries (0.7 vs 0.5).
	if reg.Confidence != 0.7 {
		t.Errorf("expected regression confidence capped at 0.7, got %v", reg.Confidence)
	}
	if len(reg.Evidence) == 0 {
		t.Errorf("expected regression to carry evidence")
	}

	saf, ok := byTag["safari"]
	if !ok {
		t.Fatalf("expected safari to be present (the evidenced entry)")
	}
	if saf.Confidence != 0.7 {
		t.Errorf("expected safari confidence capped at 0.7, got %v", saf.Confidence)
	}
}

func TestNormalizeNegatedEmptyInput(t *testing.T) {
	if got := normalizeNegated(nil, []Tag{{Name: "bug"}}); got != nil {
		t.Errorf("expected nil for nil input, got %+v", got)
	}
}

func TestScoreTagSpecificityWithStub(t *testing.T) {
	analyzer := NewAnalyzer(NewStubTagger(), NewStubEmbedder())

	catalog := []Tag{
		{Name: "backend"},
		{Name: "safari"},
		{Name: "safari-flexbox-gap"},
		{Name: "onboarding"},
	}

	tests := []struct {
		tag      Tag
		wantLow  float64
		wantHigh float64
	}{
		{Tag{Name: "backend"}, 0.0, 0.3},
		{Tag{Name: "safari"}, 0.4, 0.6},
		{Tag{Name: "safari-flexbox-gap"}, 0.6, 1.0},
	}

	for _, tt := range tests {
		score, err := analyzer.ScoreTagSpecificity(context.Background(), tt.tag, catalog)
		if err != nil {
			t.Fatalf("ScoreTagSpecificity(%q): %v", tt.tag.Name, err)
		}
		if score < tt.wantLow || score > tt.wantHigh {
			t.Errorf("ScoreTagSpecificity(%q) = %v, want [%v, %v]", tt.tag.Name, score, tt.wantLow, tt.wantHigh)
		}
	}
}
