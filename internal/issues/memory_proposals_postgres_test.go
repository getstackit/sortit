package issues

import (
	"context"
	"errors"
	"testing"

	"sortit/internal/domain"
)

func TestPostgresStoreMemoryProposalCRUD(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	proposal := domain.MemoryProposal{
		ID:             "prop-1",
		Title:          "Decisions about search",
		Body:           "Search defaults to ridge.",
		Kind:           domain.MemoryKindDecision,
		AnchorTags:     []string{"search"},
		SourceIssueIDs: []string{"i1", "i2"},
		Confidence:     0.49,
		Status:         domain.MemoryProposalStatusPending,
		Rationale:      "Synthesized from 2 closed decisions tagged \"search\"",
	}
	if err := store.UpsertMemoryProposal(ctx, proposal); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.GetMemoryProposal(ctx, "prop-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != proposal.Title || len(got.SourceIssueIDs) != 2 || got.Confidence != 0.49 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Status != domain.MemoryProposalStatusPending {
		t.Fatalf("expected pending status, got %s", got.Status)
	}

	// Resolve it.
	got.Status = domain.MemoryProposalStatusAccepted
	got.AcceptedMemoryID = "mem-1"
	if err := store.UpsertMemoryProposal(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}

	pending, err := store.ListMemoryProposals(ctx, domain.MemoryProposalStatusPending)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending proposals after acceptance, got %d", len(pending))
	}
	accepted, err := store.ListMemoryProposals(ctx, domain.MemoryProposalStatusAccepted)
	if err != nil {
		t.Fatalf("list accepted: %v", err)
	}
	if len(accepted) != 1 || accepted[0].AcceptedMemoryID != "mem-1" {
		t.Fatalf("expected 1 accepted proposal linked to mem-1, got %+v", accepted)
	}

	if _, err := store.GetMemoryProposal(ctx, "missing"); !errors.Is(err, ErrMemoryProposalNotFound) {
		t.Fatalf("expected ErrMemoryProposalNotFound, got %v", err)
	}
}
