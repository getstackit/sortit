package commands

import (
	"context"
	"errors"
	"testing"

	"splat/internal/issues"
)

func TestReEnrichIssuePublishesReEnrichEventAfterCommit(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	issue := seedIssue(t, fixture.store, "Convert JSONB to pgvector", "Casey")
	handler := ReEnrichIssueHandler{
		Runner: fixture.runner,
		Store:  fixture.store,
		Events: fixture.bus,
	}

	baselineEvents, _, err := fixture.store.ListEvents(context.Background(), 20, "", "")
	if err != nil {
		t.Fatalf("list baseline events: %v", err)
	}

	reEnriched, err := handler.Handle(context.Background(), ReEnrichIssue{
		ID: issue.ID,
	})
	if err != nil {
		t.Fatalf("re-enrich issue: %v", err)
	}

	if reEnriched.EnrichmentStatus != issues.EnrichmentStatusPending {
		t.Fatalf("expected enrichment status pending, got %q", reEnriched.EnrichmentStatus)
	}

	published := assertPublishedAndPersistedEvent(t, fixture.store, fixture.bus, len(baselineEvents), "re-enrich", issue.ID)
	if published.ID == "" {
		t.Fatal("expected re-enrich event to have a non-empty ID")
	}
	if published.CreatedBy != "system" {
		t.Fatalf("expected re-enrich event created by system, got %q", published.CreatedBy)
	}
}

func TestReEnrichIssueRejectsClosedIssue(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	issue := seedIssue(t, fixture.store, "Closed issue", "Casey")
	setClosed(t, fixture.store, issue.ID, "Jordan")

	handler := ReEnrichIssueHandler{
		Runner: fixture.runner,
		Store:  fixture.store,
		Events: fixture.bus,
	}

	_, err := handler.Handle(context.Background(), ReEnrichIssue{ID: issue.ID})
	if !errors.Is(err, issues.ErrIssueClosed) {
		t.Fatalf("expected ErrIssueClosed, got %v", err)
	}
	if len(fixture.bus.events) != 0 {
		t.Fatalf("expected no published events, got %d", len(fixture.bus.events))
	}
}

func TestReEnrichIssueRejectsNotFound(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	handler := ReEnrichIssueHandler{
		Runner: fixture.runner,
		Store:  fixture.store,
		Events: fixture.bus,
	}

	_, err := handler.Handle(context.Background(), ReEnrichIssue{ID: "nonexistent"})
	if !errors.Is(err, issues.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReEnrichIssuePreservesTargetSequence(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	issue := seedIssue(t, fixture.store, "Issue with sequence", "Casey")

	// Set a specific target sequence on the issue.
	seq := 3
	if err := fixture.store.UpdateIssueFields(context.Background(), issue.ID, issues.IssueFieldUpdate{
		EnrichmentTargetSequence: &seq,
	}); err != nil {
		t.Fatalf("update target sequence: %v", err)
	}

	handler := ReEnrichIssueHandler{
		Runner: fixture.runner,
		Store:  fixture.store,
		Events: fixture.bus,
	}

	reEnriched, err := handler.Handle(context.Background(), ReEnrichIssue{ID: issue.ID})
	if err != nil {
		t.Fatalf("re-enrich issue: %v", err)
	}

	if reEnriched.EnrichmentTargetSequence != 3 {
		t.Fatalf("expected target sequence preserved at 3, got %d", reEnriched.EnrichmentTargetSequence)
	}
}
