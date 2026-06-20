package curation

import (
	"context"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/issues/commands"
	"sortit/internal/memories"
)

type noopEvents struct{}

func (noopEvents) Publish(context.Context, issues.Event)    {}
func (noopEvents) PublishOne(context.Context, issues.Event) {}

// TestHandlerDispatcherReenrichEndToEnd wires the real ReEnrichIssueHandler
// through the HandlerDispatcher and confirms accepting a proposal actually
// re-queues enrichment for the target issue.
func TestHandlerDispatcherReenrichEndToEnd(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := issues.NewInMemoryStore([]issues.Issue{{
		ID:     "i1",
		Raw:    "Safari crashes when exporting a PDF",
		Status: issues.StatusOpen,
	}})
	runner := &commands.CommandRunner{DB: store}
	dispatcher := HandlerDispatcher{
		ReenrichHandler: commands.ReEnrichIssueHandler{Runner: runner, Store: store, Events: noopEvents{}},
	}
	svc := NewService(store, dispatcher, nil)

	p, err := svc.CreateProposal(ctx, CreateProposalInput{
		Kind:    domain.CurationKindReenrichIssue,
		Payload: domain.CurationPayload{IssueID: "i1"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	accepted, err := svc.AcceptProposal(ctx, p.ID, "reviewer")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if accepted.ResultRef != "i1" {
		t.Fatalf("expected result ref i1, got %q", accepted.ResultRef)
	}

	got, err := store.Get(ctx, "i1")
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if got.EnrichmentStatus != issues.EnrichmentStatusPending {
		t.Fatalf("expected issue re-queued (pending), got %q", got.EnrichmentStatus)
	}
}

// TestHandlerDispatcherArchiveMemoryEndToEnd wires the real memories.Service
// through the HandlerDispatcher and confirms accepting an archive proposal
// transitions the memory to archived.
func TestHandlerDispatcherArchiveMemoryEndToEnd(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := issues.NewInMemoryStore(nil)
	memSvc := memories.NewService(store, nil, nil) // nil enricher: persists without scores
	mem, err := memSvc.CreateMemory(ctx, memories.CreateMemoryInput{
		Title: "Use ridge GCV for search",
		Body:  "We chose GCV lambda selection for search ranking.",
		Kind:  domain.MemoryKindDecision,
	})
	if err != nil {
		t.Fatalf("create memory: %v", err)
	}

	dispatcher := HandlerDispatcher{Memories: memSvc}
	svc := NewService(store, dispatcher, nil)

	p, err := svc.CreateProposal(ctx, CreateProposalInput{
		Kind:    domain.CurationKindArchiveMemory,
		Payload: domain.CurationPayload{MemoryID: mem.ID},
	})
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if _, err := svc.AcceptProposal(ctx, p.ID, "reviewer"); err != nil {
		t.Fatalf("accept: %v", err)
	}

	got, err := memSvc.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got.Status != domain.MemoryStatusArchived {
		t.Fatalf("expected memory archived, got %q", got.Status)
	}
}
