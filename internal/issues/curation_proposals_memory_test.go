package issues

import (
	"context"
	"errors"
	"testing"

	"sortit/internal/domain"
)

func TestInMemoryCurationProposalsRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewInMemoryStore(nil)

	pending := domain.CurationProposal{
		ID:         "p-pending",
		Kind:       domain.CurationKindReenrichIssue,
		Payload:    domain.CurationPayload{IssueID: "i1"},
		Status:     domain.CurationProposalStatusPending,
		SourceRefs: []string{"i1"},
	}
	accepted := domain.CurationProposal{
		ID:      "p-accepted",
		Kind:    domain.CurationKindArchiveMemory,
		Payload: domain.CurationPayload{MemoryID: "m1"},
		Status:  domain.CurationProposalStatusAccepted,
	}
	for _, p := range []domain.CurationProposal{pending, accepted} {
		if err := store.UpsertCurationProposal(ctx, p); err != nil {
			t.Fatalf("upsert %s: %v", p.ID, err)
		}
	}

	all, err := store.ListCurationProposals(ctx, "")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(all))
	}

	onlyPending, err := store.ListCurationProposals(ctx, domain.CurationProposalStatusPending)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(onlyPending) != 1 || onlyPending[0].ID != "p-pending" {
		t.Fatalf("expected only p-pending, got %+v", onlyPending)
	}

	// Mutating the returned slice must not leak into the store (clone isolation).
	onlyPending[0].Payload.IssueID = "tampered"
	again, err := store.GetCurationProposal(ctx, "p-pending")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if again.Payload.IssueID != "i1" {
		t.Fatalf("expected stored payload untouched, got %q", again.Payload.IssueID)
	}

	if _, err := store.GetCurationProposal(ctx, "missing"); !errors.Is(err, ErrCurationProposalNotFound) {
		t.Fatalf("expected ErrCurationProposalNotFound, got %v", err)
	}
}
