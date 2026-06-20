package domain

import "time"

// CurationKind identifies which curation move a CurationProposal represents.
// Each kind maps to an existing command handler when the proposal is accepted.
type CurationKind string

const (
	// CurationKindCombineIssues merges two or more duplicate issues into a new
	// combined issue (the sources are closed as merged_into).
	CurationKindCombineIssues CurationKind = "combine_issues"
	// CurationKindCloseStale closes a long-inactive open issue with reason stale.
	CurationKindCloseStale CurationKind = "close_stale"
	// CurationKindReenrichIssue re-runs enrichment for an issue whose tagging or
	// embedding is missing, failed, or low-quality.
	CurationKindReenrichIssue CurationKind = "reenrich_issue"
	// CurationKindArchiveMemory retires a quiet, un-reinforced memory.
	CurationKindArchiveMemory CurationKind = "archive_memory"
	// CurationKindSupersedeMemory marks a redundant memory as superseded by a
	// replacement.
	CurationKindSupersedeMemory CurationKind = "supersede_memory"
)

// CurationProposalStatus tracks a curation move through human review.
type CurationProposalStatus string

const (
	CurationProposalStatusPending  CurationProposalStatus = "pending"
	CurationProposalStatusAccepted CurationProposalStatus = "accepted"
	CurationProposalStatusRejected CurationProposalStatus = "rejected"
)

// CurationPayload is the per-kind target reference for a curation move. Only the
// fields relevant to the proposal's Kind are populated; it serializes as a
// single JSONB column so new kinds never require a schema change.
type CurationPayload struct {
	// combine_issues
	IssueIDs []string `json:"issueIds,omitempty"`
	// CanonicalID is an advisory hint about which source the librarian considers
	// canonical. The combine handler always mints a new combined issue, so this
	// is metadata for reviewers, not an instruction the dispatcher honors.
	CanonicalID string `json:"canonicalId,omitempty"`

	// close_stale / reenrich_issue
	IssueID string `json:"issueId,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Note    string `json:"note,omitempty"`

	// archive_memory / supersede_memory
	MemoryID string `json:"memoryId,omitempty"`
	// ReplacementID is the memory that supersedes MemoryID (supersede only).
	ReplacementID string `json:"replacementId,omitempty"`
}

// CurationProposal is a single curation move drafted by the librarian (a local,
// codebase-aware agent) and held in a review queue. The system proposes; a human
// accepts or rejects. Accepting dispatches to the underlying command handler.
type CurationProposal struct {
	ID      string                 `json:"id"`
	Kind    CurationKind           `json:"kind"`
	Payload CurationPayload        `json:"payload"`
	Status  CurationProposalStatus `json:"status"`
	// Rationale explains why the move was proposed (e.g. "3 issues with >0.9
	// similarity all describe the same Safari export crash").
	Rationale  string  `json:"rationale,omitempty"`
	Confidence float64 `json:"confidence"`
	// SourceRefs are the issue/memory ids cited as evidence for the move.
	SourceRefs []string `json:"sourceRefs,omitempty"`
	// CreatedBy is the actor that drafted the proposal ("librarian" or a person).
	CreatedBy string `json:"createdBy,omitempty"`
	// ReviewedBy is the actor that accepted or rejected the proposal.
	ReviewedBy string `json:"reviewedBy,omitempty"`
	// ResultRef records what acceptance produced (e.g. the new combined issue id).
	ResultRef string    `json:"resultRef,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NormalizeCurationKind coerces a kind to a known value, returning "" when
// unrecognized so callers can reject it explicitly.
func NormalizeCurationKind(kind CurationKind) CurationKind {
	switch kind {
	case CurationKindCombineIssues,
		CurationKindCloseStale,
		CurationKindReenrichIssue,
		CurationKindArchiveMemory,
		CurationKindSupersedeMemory:
		return kind
	default:
		return ""
	}
}

// NormalizeCurationProposalStatus coerces a status to a known value, defaulting
// to pending.
func NormalizeCurationProposalStatus(status CurationProposalStatus) CurationProposalStatus {
	switch status {
	case CurationProposalStatusPending, CurationProposalStatusAccepted, CurationProposalStatusRejected:
		return status
	default:
		return CurationProposalStatusPending
	}
}

// CloneCurationProposal returns a deep copy so callers can't mutate stored slices.
func CloneCurationProposal(p CurationProposal) CurationProposal {
	out := p
	if len(p.Payload.IssueIDs) > 0 {
		out.Payload.IssueIDs = append([]string(nil), p.Payload.IssueIDs...)
	}
	if len(p.SourceRefs) > 0 {
		out.SourceRefs = append([]string(nil), p.SourceRefs...)
	}
	return out
}
