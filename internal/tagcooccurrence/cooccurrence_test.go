package tagcooccurrence

import (
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

func TestComputeEmptyCorpus(t *testing.T) {
	got := Compute(nil)
	if got.IssueCount != 0 || got.TagCount != 0 || len(got.Tags) != 0 {
		t.Fatalf("expected empty stats, got %+v", got)
	}
}

func TestComputeIgnoresBelowFloor(t *testing.T) {
	items := []issues.Issue{
		{TagScores: []domain.TagRelevance{
			{Tag: "bug", Relevance: 0.9},
			{Tag: "noise", Relevance: 0.05}, // below floor — should be ignored
		}},
	}
	got := Compute(items)
	if got.TagCount != 1 {
		t.Fatalf("expected 1 tag above floor, got %d: %+v", got.TagCount, got)
	}
	if got.Tags[0].Tag != "bug" {
		t.Fatalf("expected bug, got %q", got.Tags[0].Tag)
	}
}

// issuesWithTags builds n issues each carrying the given tags at strong
// relevance.
func issuesWithTags(n int, tags ...string) []issues.Issue {
	out := make([]issues.Issue, 0, n)
	for range n {
		scores := make([]domain.TagRelevance, 0, len(tags))
		for _, tag := range tags {
			scores = append(scores, domain.TagRelevance{Tag: tag, Relevance: 0.9})
		}
		out = append(out, issues.Issue{TagScores: scores})
	}
	return out
}

func TestComputeSmallCorpusEmitsNoAntiCorrelation(t *testing.T) {
	// bug and feature never co-occur, but bug appears on only 3 issues —
	// below AntiCorrelationMinPresence — so the guards force the
	// implicit-negative to zero instead of flagging spurious exclusion.
	items := []issues.Issue{
		{TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.9}}},
		{TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.9}}},
		{TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.9}}},
		{TagScores: []domain.TagRelevance{{Tag: "feature", Relevance: 0.9}}},
	}
	got := Compute(items)
	bug, ok := got.LookupTag("bug")
	if !ok {
		t.Fatalf("expected bug stats")
	}
	if bug.Count != 3 {
		t.Fatalf("expected bug count 3, got %d", bug.Count)
	}
	if len(bug.Pairs) != 1 || bug.Pairs[0].Tag != "feature" {
		t.Fatalf("expected single pair against feature, got %+v", bug.Pairs)
	}
	if bug.Pairs[0].ImplicitNegative != 0 {
		t.Fatalf("expected implicit-negative 0 on small corpus, got %v", bug.Pairs[0].ImplicitNegative)
	}
	if bug.Pairs[0].ConditionalProb != 0 {
		t.Fatalf("expected conditional 0, got %v", bug.Pairs[0].ConditionalProb)
	}
}

func TestComputeLargeCorpusAntiCorrelation(t *testing.T) {
	// 100-issue corpus: bug appears on 40 issues (always with crash),
	// feature on 30, misc on 32. bug and feature overlap on only 2 issues.
	items := make([]issues.Issue, 0, 100)
	items = append(items, issuesWithTags(38, "bug", "crash")...)
	items = append(items, issuesWithTags(2, "bug", "crash", "feature")...)
	items = append(items, issuesWithTags(28, "feature")...)
	items = append(items, issuesWithTags(32, "misc")...)

	got := Compute(items)
	bug, ok := got.LookupTag("bug")
	if !ok {
		t.Fatalf("expected bug stats")
	}
	if bug.Count != 40 {
		t.Fatalf("expected bug count 40, got %d", bug.Count)
	}

	pairByTag := make(map[string]PairStats, len(bug.Pairs))
	for _, p := range bug.Pairs {
		pairByTag[p.Tag] = p
	}

	// bug → feature clears both floors (count 40 ≥ 20; expected joint
	// 40 × 0.3 = 12 ≥ 5). Shrunk lift = 1 − (2+1)/(12+1) ≈ 0.77.
	feature := pairByTag["feature"]
	if feature.ImplicitNegative != 0.77 {
		t.Fatalf("expected implicit-negative 0.77 for bug→feature, got %v", feature.ImplicitNegative)
	}
	if feature.JointCount != 2 {
		t.Fatalf("expected joint count 2, got %d", feature.JointCount)
	}

	// bug → crash always co-occur: joint 40 exceeds expected 16, lift goes
	// negative and clamps to zero.
	crash := pairByTag["crash"]
	if crash.ImplicitNegative != 0 {
		t.Fatalf("expected implicit-negative 0 for fully-correlated bug→crash, got %v", crash.ImplicitNegative)
	}
	if crash.ConditionalProb != 1 {
		t.Fatalf("expected conditional 1 for bug→crash, got %v", crash.ConditionalProb)
	}
}

func TestComputeExpectedJointFloor(t *testing.T) {
	// alpha appears on 25 issues (above the presence floor), but beta is so
	// rare (8/100) that independence predicts only 25 × 0.08 = 2 joint
	// occurrences — below AntiCorrelationMinExpectedJoint. Observing zero
	// carries no evidence, so the signal stays zero.
	items := make([]issues.Issue, 0, 100)
	items = append(items, issuesWithTags(25, "alpha")...)
	items = append(items, issuesWithTags(8, "beta")...)
	items = append(items, issuesWithTags(67, "filler")...)

	got := Compute(items)
	alpha, ok := got.LookupTag("alpha")
	if !ok {
		t.Fatalf("expected alpha stats")
	}
	for _, p := range alpha.Pairs {
		if p.Tag == "beta" && p.ImplicitNegative != 0 {
			t.Fatalf("expected implicit-negative 0 for alpha→beta below expected-joint floor, got %v", p.ImplicitNegative)
		}
	}
}

func TestRound2RoundsNegativesAwayFromZero(t *testing.T) {
	// The previous int-truncation implementation rounded -0.125 to -0.12.
	if got := round2(-0.125); got != -0.13 {
		t.Fatalf("round2(-0.125) = %v, want -0.13", got)
	}
	if got := round2(0.125); got != 0.13 {
		t.Fatalf("round2(0.125) = %v, want 0.13", got)
	}
}

func TestComputeNormalizesTagNames(t *testing.T) {
	items := []issues.Issue{
		{TagScores: []domain.TagRelevance{{Tag: " Bug ", Relevance: 0.9}}},
		{TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.9}}},
	}
	got := Compute(items)
	if got.TagCount != 1 {
		t.Fatalf("expected normalized to 1 tag, got %d: %+v", got.TagCount, got)
	}
	if got.Tags[0].Count != 2 {
		t.Fatalf("expected normalized count 2, got %d", got.Tags[0].Count)
	}
}
