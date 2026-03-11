package commands

import (
	"context"
	"testing"

	"splat/internal/ai"
	"splat/internal/issues"
	"splat/internal/services"
)

type recordingEventBus struct {
	events []issues.Event
}

func (b *recordingEventBus) Publish(_ context.Context, event issues.Event) {
	b.events = append(b.events, event)
}

func (b *recordingEventBus) PublishOne(ctx context.Context, event issues.Event) {
	b.Publish(ctx, event)
}

func TestCreateIssuePublishesReportEventAfterCommit(t *testing.T) {
	t.Parallel()

	analyzer := ai.NewAnalyzerWithCanonicalizer(
		ai.NewStubTagger(),
		ai.NewStubEmbedder(),
		ai.NewStubCanonicalizer(),
	)
	store := issues.NewInMemoryStore(nil)
	catalog := services.NewCatalogService(nil, analyzer)
	handler := CreateIssueHandler{
		Runner:   &CommandRunner{DB: store},
		Enricher: services.NewIssueEnricher(analyzer, catalog),
		Events:   &recordingEventBus{},
	}

	bus := handler.Events.(*recordingEventBus)
	created, err := handler.Handle(context.Background(), CreateIssue{
		Raw:       "Safari crashes when exporting a PDF",
		CreatedBy: "Casey",
	})
	if err != nil {
		t.Fatalf("handle create issue: %v", err)
	}

	if len(bus.events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(bus.events))
	}

	published := bus.events[0]
	if published.Kind != "report" {
		t.Fatalf("expected report event, got %q", published.Kind)
	}
	if published.IssueID != created.ID {
		t.Fatalf("expected published event for issue %q, got %q", created.ID, published.IssueID)
	}

	events, _, err := store.ListEvents(context.Background(), 10, "", "")
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 persisted event, got %d", len(events))
	}
	if events[0].ID != published.ID {
		t.Fatalf("expected published event %q to match persisted event %q", published.ID, events[0].ID)
	}
}
