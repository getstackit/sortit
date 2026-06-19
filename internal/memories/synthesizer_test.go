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
