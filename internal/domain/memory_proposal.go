package domain

import "time"

// MemoryProposalStatus tracks a synthesized memory through human review.
type MemoryProposalStatus string

const (
	MemoryProposalStatusPending  MemoryProposalStatus = "pending"
	MemoryProposalStatusAccepted MemoryProposalStatus = "accepted"
	MemoryProposalStatusRejected MemoryProposalStatus = "rejected"
)

// MemoryProposal is a candidate memory synthesized from the corpus (matured
// regions, clusters of closed decisions) that awaits human acceptance before
// becoming a permanent Memory. The hybrid creation model keeps quality high:
// the system proposes, a human confirms.
type MemoryProposal struct {
	ID             string               `json:"id"`
	Title          string               `json:"title"`
	Body           string               `json:"body"`
	Kind           MemoryKind           `json:"kind"`
	AnchorTags     []string             `json:"anchorTags,omitempty"`
	AnchorRegion   string               `json:"anchorRegion,omitempty"`
	SourceIssueIDs []string             `json:"sourceIssueIds,omitempty"`
	Confidence     float64              `json:"confidence"`
	Status         MemoryProposalStatus `json:"status"`
	// Rationale explains why this was synthesized (e.g. "5 closed decisions
	// tagged search").
	Rationale string `json:"rationale,omitempty"`
	// AcceptedMemoryID is the id of the Memory created when this proposal was
	// accepted.
	AcceptedMemoryID string    `json:"acceptedMemoryId,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// NormalizeMemoryProposalStatus coerces a status to a known value, defaulting
// to pending.
func NormalizeMemoryProposalStatus(status MemoryProposalStatus) MemoryProposalStatus {
	switch status {
	case MemoryProposalStatusPending, MemoryProposalStatusAccepted, MemoryProposalStatusRejected:
		return status
	default:
		return MemoryProposalStatusPending
	}
}

// CloneMemoryProposal returns a deep copy so callers can't mutate stored slices.
func CloneMemoryProposal(p MemoryProposal) MemoryProposal {
	out := p
	if len(p.AnchorTags) > 0 {
		out.AnchorTags = append([]string(nil), p.AnchorTags...)
	}
	if len(p.SourceIssueIDs) > 0 {
		out.SourceIssueIDs = append([]string(nil), p.SourceIssueIDs...)
	}
	return out
}
