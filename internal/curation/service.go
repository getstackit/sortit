// Package curation provides the librarian review queue: a propose-only surface
// where a local, codebase-aware agent drafts curation moves (combine duplicate
// issues, close stale ones, re-enrich, archive/supersede memories) and a human
// accepts or rejects each. Accepting dispatches to the existing command
// handlers. Candidate detection (what's worth proposing) lives alongside in
// detect.go; the backend never acts on its own.
package curation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/issues/commands"
)

// ErrProposalNotPending is returned when accepting or rejecting a proposal that
// has already been resolved.
var ErrProposalNotPending = errors.New("curation proposal is not pending")

// ErrUnknownKind is returned when a proposal's kind is not one of the supported
// curation moves.
var ErrUnknownKind = errors.New("unknown curation kind")

// ErrInvalidPayload is returned when a proposal's payload doesn't carry the
// references its kind requires.
var ErrInvalidPayload = errors.New("invalid curation payload")

// defaultCloseReason is used when a close_stale proposal omits an explicit reason.
const defaultCloseReason = "stale"

// Service is the application layer for curation proposals.
type Service struct {
	proposals  issues.CurationProposalStore
	dispatcher Dispatcher
	logger     *slog.Logger
}

// NewService constructs a curation service.
func NewService(proposals issues.CurationProposalStore, dispatcher Dispatcher, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		proposals:  proposals,
		dispatcher: dispatcher,
		logger:     logger.With("component", "curation"),
	}
}

// ListProposals returns curation proposals, optionally filtered by status.
func (s *Service) ListProposals(ctx context.Context, status domain.CurationProposalStatus) ([]domain.CurationProposal, error) {
	return s.proposals.ListCurationProposals(ctx, status)
}

// GetProposal returns one proposal or issues.ErrCurationProposalNotFound.
func (s *Service) GetProposal(ctx context.Context, id string) (domain.CurationProposal, error) {
	return s.proposals.GetCurationProposal(ctx, id)
}

// CreateProposalInput describes a curation move drafted by the librarian.
type CreateProposalInput struct {
	Kind       domain.CurationKind
	Payload    domain.CurationPayload
	Rationale  string
	Confidence float64
	SourceRefs []string
	CreatedBy  string
}

// CreateProposal validates and persists a new pending proposal. Detection lives
// in the librarian agent; this only records the move it decided to propose.
func (s *Service) CreateProposal(ctx context.Context, input CreateProposalInput) (domain.CurationProposal, error) {
	kind := domain.NormalizeCurationKind(input.Kind)
	if kind == "" {
		return domain.CurationProposal{}, fmt.Errorf("%w: %q", ErrUnknownKind, input.Kind)
	}
	payload, err := normalizePayload(kind, input.Payload)
	if err != nil {
		return domain.CurationProposal{}, err
	}

	now := time.Now().UTC()
	proposal := domain.CurationProposal{
		ID:         issues.NewCurationProposalID(),
		Kind:       kind,
		Payload:    payload,
		Status:     domain.CurationProposalStatusPending,
		Rationale:  strings.TrimSpace(input.Rationale),
		Confidence: input.Confidence,
		SourceRefs: normalizeIDList(input.SourceRefs),
		CreatedBy:  strings.TrimSpace(input.CreatedBy),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.proposals.UpsertCurationProposal(ctx, proposal); err != nil {
		return domain.CurationProposal{}, fmt.Errorf("save curation proposal: %w", err)
	}
	s.logger.InfoContext(ctx, "curation proposal created",
		"proposal_id", proposal.ID,
		"kind", proposal.Kind,
		"created_by", proposal.CreatedBy,
	)
	return proposal, nil
}

// RejectProposal marks a pending proposal as rejected.
func (s *Service) RejectProposal(ctx context.Context, id, reviewedBy string) (domain.CurationProposal, error) {
	proposal, err := s.proposals.GetCurationProposal(ctx, id)
	if err != nil {
		return domain.CurationProposal{}, err
	}
	if proposal.Status != domain.CurationProposalStatusPending {
		return domain.CurationProposal{}, ErrProposalNotPending
	}
	proposal.Status = domain.CurationProposalStatusRejected
	proposal.ReviewedBy = strings.TrimSpace(reviewedBy)
	proposal.UpdatedAt = time.Now().UTC()
	if err := s.proposals.UpsertCurationProposal(ctx, proposal); err != nil {
		return domain.CurationProposal{}, fmt.Errorf("mark proposal rejected: %w", err)
	}
	return proposal, nil
}

// AcceptProposal applies a pending proposal by dispatching to the underlying
// command handler, then marks it accepted and records the result reference.
//
// Each handler commits its own unit of work; the proposal-status write is a
// separate commit (same non-atomic residual as the memory accept path). The
// handler is invoked first and any error leaves the proposal pending — so a
// retried accept of an idempotent kind (close, re-enrich, archive) is safe.
// combine_issues is NOT idempotent: a retry after a successful dispatch but
// failed status write would mint a second combined issue. Reviewers should not
// re-accept a combine whose action already applied.
func (s *Service) AcceptProposal(ctx context.Context, id, reviewedBy string) (domain.CurationProposal, error) {
	proposal, err := s.proposals.GetCurationProposal(ctx, id)
	if err != nil {
		return domain.CurationProposal{}, err
	}
	if proposal.Status != domain.CurationProposalStatusPending {
		return domain.CurationProposal{}, ErrProposalNotPending
	}
	reviewer := strings.TrimSpace(reviewedBy)

	resultRef, err := s.dispatch(ctx, proposal, reviewer)
	if err != nil {
		return domain.CurationProposal{}, fmt.Errorf("apply curation move: %w", err)
	}

	proposal.Status = domain.CurationProposalStatusAccepted
	proposal.ReviewedBy = reviewer
	proposal.ResultRef = resultRef
	proposal.UpdatedAt = time.Now().UTC()
	if err := s.proposals.UpsertCurationProposal(ctx, proposal); err != nil {
		return domain.CurationProposal{}, fmt.Errorf("mark proposal accepted: %w", err)
	}
	s.logger.InfoContext(ctx, "curation proposal accepted",
		"proposal_id", proposal.ID,
		"kind", proposal.Kind,
		"result_ref", resultRef,
	)
	return proposal, nil
}

// dispatch routes an accepted proposal to the right command handler and returns
// the reference to whatever it produced (new combined issue id, closed/enriched
// issue id, or transitioned memory id).
func (s *Service) dispatch(ctx context.Context, proposal domain.CurationProposal, reviewer string) (string, error) {
	p := proposal.Payload
	switch proposal.Kind {
	case domain.CurationKindCombineIssues:
		result, err := s.dispatcher.Combine(ctx, commands.CombineIssues{
			SourceIDs: p.IssueIDs,
			CreatedBy: reviewer,
			Note:      proposal.Rationale,
		})
		if err != nil {
			return "", err
		}
		if len(result.CreatedIssues) == 0 {
			return "", nil
		}
		return result.CreatedIssues[0].ID, nil

	case domain.CurationKindCloseStale:
		reason := strings.TrimSpace(p.Reason)
		if reason == "" {
			reason = defaultCloseReason
		}
		closed, err := s.dispatcher.Close(ctx, commands.CloseIssue{
			ID:         p.IssueID,
			ClosedBy:   reviewer,
			Reason:     reason,
			ReasonNote: p.Note,
		})
		if err != nil {
			return "", err
		}
		return closed.ID, nil

	case domain.CurationKindReenrichIssue:
		reEnriched, err := s.dispatcher.Reenrich(ctx, commands.ReEnrichIssue{ID: p.IssueID})
		if err != nil {
			return "", err
		}
		return reEnriched.ID, nil

	case domain.CurationKindArchiveMemory:
		mem, err := s.dispatcher.ArchiveMemory(ctx, p.MemoryID)
		if err != nil {
			return "", err
		}
		return mem.ID, nil

	case domain.CurationKindSupersedeMemory:
		mem, err := s.dispatcher.SupersedeMemory(ctx, p.MemoryID, p.ReplacementID)
		if err != nil {
			return "", err
		}
		return mem.ID, nil

	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownKind, proposal.Kind)
	}
}

// normalizePayload validates and trims a payload for its kind, keeping only the
// fields that kind uses.
func normalizePayload(kind domain.CurationKind, in domain.CurationPayload) (domain.CurationPayload, error) {
	switch kind {
	case domain.CurationKindCombineIssues:
		ids := issues.SanitizeIssueIDs(in.IssueIDs)
		if len(ids) < 2 {
			return domain.CurationPayload{}, fmt.Errorf("%w: combine_issues requires at least two issueIds", ErrInvalidPayload)
		}
		return domain.CurationPayload{
			IssueIDs:    ids,
			CanonicalID: strings.TrimSpace(in.CanonicalID),
		}, nil

	case domain.CurationKindCloseStale:
		id := strings.TrimSpace(in.IssueID)
		if id == "" {
			return domain.CurationPayload{}, fmt.Errorf("%w: close_stale requires issueId", ErrInvalidPayload)
		}
		reason := strings.TrimSpace(in.Reason)
		if reason == "" {
			reason = defaultCloseReason
		}
		if err := issues.ValidateCloseReason(reason); err != nil {
			return domain.CurationPayload{}, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
		}
		return domain.CurationPayload{IssueID: id, Reason: reason, Note: strings.TrimSpace(in.Note)}, nil

	case domain.CurationKindReenrichIssue:
		id := strings.TrimSpace(in.IssueID)
		if id == "" {
			return domain.CurationPayload{}, fmt.Errorf("%w: reenrich_issue requires issueId", ErrInvalidPayload)
		}
		return domain.CurationPayload{IssueID: id}, nil

	case domain.CurationKindArchiveMemory:
		id := strings.TrimSpace(in.MemoryID)
		if id == "" {
			return domain.CurationPayload{}, fmt.Errorf("%w: archive_memory requires memoryId", ErrInvalidPayload)
		}
		return domain.CurationPayload{MemoryID: id}, nil

	case domain.CurationKindSupersedeMemory:
		id := strings.TrimSpace(in.MemoryID)
		if id == "" {
			return domain.CurationPayload{}, fmt.Errorf("%w: supersede_memory requires memoryId", ErrInvalidPayload)
		}
		replacement := strings.TrimSpace(in.ReplacementID)
		if replacement == id {
			return domain.CurationPayload{}, fmt.Errorf("%w: a memory cannot supersede itself", ErrInvalidPayload)
		}
		return domain.CurationPayload{MemoryID: id, ReplacementID: replacement}, nil

	default:
		return domain.CurationPayload{}, fmt.Errorf("%w: %q", ErrUnknownKind, kind)
	}
}

func normalizeIDList(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, id := range in {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
