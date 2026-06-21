package memories

import (
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

const searchTag = "search"

func closedDecision(id, tag, note, reason string) issues.Issue {
	return issues.Issue{
		ID:               id,
		Status:           issues.StatusClosed,
		ClosedReason:     reason,
		ClosedReasonNote: note,
		Raw:              id + " body",
		Tags:             []string{tag},
		TagScores:        []domain.TagRelevance{{Tag: tag, Relevance: 0.9}},
	}
}

func TestSynthesizeMemoryProposals(t *testing.T) {
	closed := []issues.Issue{
		closedDecision("i1", searchTag, "default to ridge", "by_design"),
		closedDecision("i2", searchTag, "", "by_design"),
		closedDecision("i3", "backend", "single decision", "fixed"),
		{ID: "i4", Status: issues.StatusOpen, Tags: []string{searchTag}, TagScores: []domain.TagRelevance{{Tag: searchTag, Relevance: 0.9}}},
	}

	drafts := SynthesizeMemoryProposals(closed, nil, nil)
	if len(drafts) != 1 {
		t.Fatalf("expected 1 draft (search has 2 decisions, backend only 1), got %d: %+v", len(drafts), drafts)
	}
	draft := drafts[0]
	if len(draft.AnchorTags) != 1 || draft.AnchorTags[0] != searchTag {
		t.Fatalf("expected search anchor, got %+v", draft.AnchorTags)
	}
	if len(draft.SourceIssueIDs) != 2 {
		t.Fatalf("expected 2 source issues, got %+v", draft.SourceIssueIDs)
	}
	if draft.Status != domain.MemoryProposalStatusPending {
		t.Fatalf("expected pending status, got %s", draft.Status)
	}
	if draft.Confidence <= 0 || draft.Confidence > 1 {
		t.Fatalf("confidence out of range: %v", draft.Confidence)
	}
}

func taggedIssue(id, tag string) issues.Issue {
	return issues.Issue{
		ID:        id,
		Status:    issues.StatusOpen,
		Raw:       id + " body",
		Tags:      []string{tag},
		TagScores: []domain.TagRelevance{{Tag: tag, Relevance: 0.8}},
	}
}

func TestSynthesizeConceptProposals(t *testing.T) {
	corpus := make([]issues.Issue, 0, 7)
	for i := range 6 { // 6 issues carry "ridge" → load-bearing
		corpus = append(corpus, taggedIssue("r"+string(rune('0'+i)), "ridge"))
	}
	corpus = append(corpus, taggedIssue("b1", "backend")) // only 1 → below threshold

	drafts := SynthesizeConceptProposals(corpus, nil, nil, nil)
	if len(drafts) != 1 {
		t.Fatalf("expected 1 concept draft (ridge has 6, backend 1), got %d: %+v", len(drafts), drafts)
	}
	d := drafts[0]
	if d.Kind != domain.MemoryKindConcept {
		t.Fatalf("expected concept kind, got %s", d.Kind)
	}
	if d.SubjectTag != "ridge" {
		t.Fatalf("expected subject tag ridge, got %q", d.SubjectTag)
	}
	if len(d.AnchorTags) != 1 || d.AnchorTags[0] != "ridge" {
		t.Fatalf("expected ridge anchor, got %+v", d.AnchorTags)
	}
	if len(d.SourceIssueIDs) != 6 {
		t.Fatalf("expected 6 source issues, got %d", len(d.SourceIssueIDs))
	}

	// A tag that already has an active concept is skipped (idempotent reruns).
	covered := SynthesizeConceptProposals(corpus, nil, []domain.Memory{
		{Kind: domain.MemoryKindConcept, Status: domain.MemoryStatusActive, SubjectTag: "ridge"},
	}, nil)
	if len(covered) != 0 {
		t.Fatalf("expected ridge concept skipped when already covered, got %+v", covered)
	}

	// A pending concept proposal also covers the tag.
	coveredPending := SynthesizeConceptProposals(corpus, []domain.MemoryProposal{
		{Kind: domain.MemoryKindConcept, SubjectTag: "ridge"},
	}, nil, nil)
	if len(coveredPending) != 0 {
		t.Fatalf("expected ridge concept skipped when a pending proposal exists, got %+v", coveredPending)
	}
}

func TestSynthesizeConceptProposalsSpecificityGate(t *testing.T) {
	corpus := make([]issues.Issue, 0, 12)
	for i := range 6 { // 6 issues carry the specific tag
		corpus = append(corpus, taggedIssue("s"+string(rune('0'+i)), "ridge-regression"))
	}
	for i := range 6 { // 6 issues carry a generic bucket tag
		corpus = append(corpus, taggedIssue("g"+string(rune('0'+i)), "backend"))
	}

	spec := map[string]float64{
		"ridge-regression": 0.7,  // specific → eligible
		"backend":          0.13, // generic bucket → gated out
	}

	drafts := SynthesizeConceptProposals(corpus, nil, nil, spec)
	if len(drafts) != 1 {
		t.Fatalf("expected only the specific tag to yield a concept, got %d: %+v", len(drafts), drafts)
	}
	if drafts[0].SubjectTag != "ridge-regression" {
		t.Fatalf("expected ridge-regression concept, got %q", drafts[0].SubjectTag)
	}

	// Without specificity data, both qualify (gate is a no-op).
	ungated := SynthesizeConceptProposals(corpus, nil, nil, nil)
	if len(ungated) != 2 {
		t.Fatalf("expected both tags to qualify without specificity data, got %d", len(ungated))
	}
}

func TestSynthesizeMemoryProposalsSkipsCovered(t *testing.T) {
	closed := []issues.Issue{
		closedDecision("i1", searchTag, "default to ridge", "by_design"),
		closedDecision("i2", searchTag, "still ridge", "by_design"),
	}

	// Covered by an active memory.
	drafts := SynthesizeMemoryProposals(closed, nil, []domain.Memory{
		{Status: domain.MemoryStatusActive, AnchorTags: []string{searchTag}},
	})
	if len(drafts) != 0 {
		t.Fatalf("expected no drafts when an active memory covers the tag, got %d", len(drafts))
	}

	// Covered by a pending proposal.
	drafts = SynthesizeMemoryProposals(closed, []domain.MemoryProposal{
		{Status: domain.MemoryProposalStatusPending, AnchorTags: []string{searchTag}},
	}, nil)
	if len(drafts) != 0 {
		t.Fatalf("expected no drafts when a pending proposal covers the tag, got %d", len(drafts))
	}
}

func TestSynthesizeMemoryProposalsIgnoresUndocumentedClosures(t *testing.T) {
	// Closed as "fixed" with no note is not a recorded decision.
	closed := []issues.Issue{
		closedDecision("i1", searchTag, "", "fixed"),
		closedDecision("i2", searchTag, "", "fixed"),
	}
	drafts := SynthesizeMemoryProposals(closed, nil, nil)
	if len(drafts) != 0 {
		t.Fatalf("expected no drafts from undocumented closures, got %d", len(drafts))
	}
}
