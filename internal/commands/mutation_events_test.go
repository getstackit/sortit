package commands

import (
	"context"
	"testing"
	"time"

	"splat/internal/ai"
	"splat/internal/issues"
	"splat/internal/services"
)

type commandTestFixture struct {
	store    *issues.InMemoryStore
	runner   *CommandRunner
	enricher *services.IssueEnricher
	bus      *recordingEventBus
}

func newCommandTestFixture() commandTestFixture {
	analyzer := ai.NewAnalyzerWithCanonicalizer(
		ai.NewStubTagger(),
		ai.NewStubEmbedder(),
		ai.NewStubCanonicalizer(),
	)
	store := issues.NewInMemoryStore(nil)
	catalog := services.NewCatalogService(nil, analyzer)
	return commandTestFixture{
		store:    store,
		runner:   &CommandRunner{DB: store},
		enricher: services.NewIssueEnricher(analyzer, catalog),
		bus:      &recordingEventBus{},
	}
}

func seedIssue(t *testing.T, store *issues.InMemoryStore, raw, createdBy string) issues.Issue {
	t.Helper()

	issue := issues.BuildNewIssue(issues.NewIssueID(), issues.CreateInput{
		Raw:       raw,
		CreatedBy: createdBy,
	})
	if err := store.SaveIssue(context.Background(), issue); err != nil {
		t.Fatalf("seed issue: %v", err)
	}
	return issue
}

func setClosed(t *testing.T, store *issues.InMemoryStore, issueID, actor string) {
	t.Helper()

	status := issues.StatusClosed
	closedAt := time.Now().UTC()
	closedBy := issues.DefaultActor(actor)
	if err := store.UpdateIssueFields(context.Background(), issueID, issues.IssueFieldUpdate{
		Status:   &status,
		ClosedAt: &closedAt,
		ClosedBy: &closedBy,
	}); err != nil {
		t.Fatalf("close seeded issue: %v", err)
	}
}

func assertPublishedAndPersistedEvent(
	t *testing.T,
	store *issues.InMemoryStore,
	bus *recordingEventBus,
	baselineCount int,
	wantKind string,
	wantIssueID string,
) issues.Event {
	t.Helper()

	if len(bus.events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(bus.events))
	}

	published := bus.events[0]
	if published.Kind != wantKind {
		t.Fatalf("expected published event kind %q, got %q", wantKind, published.Kind)
	}
	if published.IssueID != wantIssueID {
		t.Fatalf("expected published event issue %q, got %q", wantIssueID, published.IssueID)
	}

	events, _, err := store.ListEvents(context.Background(), 20, "", "")
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != baselineCount+1 {
		t.Fatalf("expected %d persisted events, got %d", baselineCount+1, len(events))
	}
	if events[0].ID != published.ID {
		t.Fatalf("expected latest persisted event %q, got %q", published.ID, events[0].ID)
	}

	return published
}

func TestAssignIssuePublishesAssignedEventAfterCommit(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	issue := seedIssue(t, fixture.store, "Export fails in Safari", "Casey")
	handler := AssignIssueHandler{Runner: fixture.runner, Events: fixture.bus}

	baselineEvents, _, err := fixture.store.ListEvents(context.Background(), 20, "", "")
	if err != nil {
		t.Fatalf("list baseline events: %v", err)
	}

	updated, err := handler.Handle(context.Background(), AssignIssue{
		ID:         issue.ID,
		AssignedTo: "Jordan",
	})
	if err != nil {
		t.Fatalf("assign issue: %v", err)
	}
	if updated.AssignedTo != "Jordan" {
		t.Fatalf("expected issue assigned to Jordan, got %q", updated.AssignedTo)
	}

	assertPublishedAndPersistedEvent(t, fixture.store, fixture.bus, len(baselineEvents), "assigned", issue.ID)
}

func TestCloseIssuePublishesClosedEventAfterCommit(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	issue := seedIssue(t, fixture.store, "Closing regression", "Casey")
	handler := CloseIssueHandler{Runner: fixture.runner, Events: fixture.bus}

	closed, err := handler.Handle(context.Background(), CloseIssue{ID: issue.ID, ClosedBy: "Jordan"})
	if err != nil {
		t.Fatalf("close issue: %v", err)
	}
	if closed.Status != issues.StatusClosed {
		t.Fatalf("expected closed status, got %q", closed.Status)
	}

	assertPublishedAndPersistedEvent(t, fixture.store, fixture.bus, 0, "closed", issue.ID)
}

func TestCloseIssueNoOpDoesNotPublishEvent(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	issue := seedIssue(t, fixture.store, "Already closed", "Casey")
	setClosed(t, fixture.store, issue.ID, "Jordan")

	handler := CloseIssueHandler{Runner: fixture.runner, Events: fixture.bus}
	if _, err := handler.Handle(context.Background(), CloseIssue{ID: issue.ID, ClosedBy: "Taylor"}); err != nil {
		t.Fatalf("close issue noop: %v", err)
	}
	if len(fixture.bus.events) != 0 {
		t.Fatalf("expected no published events, got %d", len(fixture.bus.events))
	}
}

func TestRefineIssuePublishesRefinementEventAfterCommit(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	issue := seedIssue(t, fixture.store, "Export fails", "Casey")
	handler := RefineIssueHandler{
		Runner:   fixture.runner,
		Store:    fixture.store,
		Enricher: fixture.enricher,
		Events:   fixture.bus,
	}

	refined, err := handler.Handle(context.Background(), RefineIssue{
		ID:        issue.ID,
		Raw:       "Only happens when exporting PDFs from the share sheet.",
		CreatedBy: "Jordan",
	})
	if err != nil {
		t.Fatalf("refine issue: %v", err)
	}
	if len(refined.Discussion) != 2 {
		t.Fatalf("expected 2 discussion posts, got %d", len(refined.Discussion))
	}

	assertPublishedAndPersistedEvent(t, fixture.store, fixture.bus, 0, "refinement", issue.ID)
}

func TestProgressIssuePublishesProgressEventAfterCommit(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	issue := seedIssue(t, fixture.store, "Progress issue", "Casey")
	handler := ProgressIssueHandler{Runner: fixture.runner, Events: fixture.bus}

	progressed, err := handler.Handle(context.Background(), ProgressIssue{
		ID:        issue.ID,
		Raw:       "Added tracing to narrow the failing path.",
		CreatedBy: "Jordan",
	})
	if err != nil {
		t.Fatalf("progress issue: %v", err)
	}
	if len(progressed.Discussion) != 2 {
		t.Fatalf("expected 2 discussion posts, got %d", len(progressed.Discussion))
	}

	assertPublishedAndPersistedEvent(t, fixture.store, fixture.bus, 0, "progress", issue.ID)
}

func TestReopenIssuePublishesReopenedEventAfterCommit(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	issue := seedIssue(t, fixture.store, "Reopen me", "Casey")
	setClosed(t, fixture.store, issue.ID, "Jordan")

	handler := ReopenIssueHandler{Runner: fixture.runner, Events: fixture.bus}
	reopened, err := handler.Handle(context.Background(), ReopenIssue{ID: issue.ID})
	if err != nil {
		t.Fatalf("reopen issue: %v", err)
	}
	if reopened.Status != issues.StatusOpen {
		t.Fatalf("expected open status, got %q", reopened.Status)
	}

	assertPublishedAndPersistedEvent(t, fixture.store, fixture.bus, 0, "reopened", issue.ID)
}

func TestReopenIssueNoOpDoesNotPublishEvent(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	issue := seedIssue(t, fixture.store, "Still open", "Casey")
	handler := ReopenIssueHandler{Runner: fixture.runner, Events: fixture.bus}

	if _, err := handler.Handle(context.Background(), ReopenIssue{ID: issue.ID}); err != nil {
		t.Fatalf("reopen issue noop: %v", err)
	}
	if len(fixture.bus.events) != 0 {
		t.Fatalf("expected no published events, got %d", len(fixture.bus.events))
	}
}

func TestLinkIssuesPublishesLinkEventAfterCommit(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	source := seedIssue(t, fixture.store, "Source issue", "Casey")
	target := seedIssue(t, fixture.store, "Target issue", "Jordan")
	handler := LinkIssuesHandler{Runner: fixture.runner, Events: fixture.bus}

	result, err := handler.Handle(context.Background(), LinkIssues{
		SourceID:  source.ID,
		TargetID:  target.ID,
		Type:      issues.IssueLinkTypeRelatedTo,
		CreatedBy: "Taylor",
		Note:      "same outage",
	})
	if err != nil {
		t.Fatalf("link issues: %v", err)
	}
	if result.Operation.Kind != issues.IssueOperationKindLink {
		t.Fatalf("expected link operation, got %q", result.Operation.Kind)
	}

	assertPublishedAndPersistedEvent(t, fixture.store, fixture.bus, 0, "link", source.ID)
}

func TestCombineIssuesPublishesCombineEventAfterCommit(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	first := seedIssue(t, fixture.store, "Safari export crash", "Casey")
	second := seedIssue(t, fixture.store, "PDF export fails", "Jordan")
	handler := CombineIssuesHandler{
		Runner:   fixture.runner,
		Store:    fixture.store,
		Enricher: fixture.enricher,
		Events:   fixture.bus,
	}

	result, err := handler.Handle(context.Background(), CombineIssues{
		SourceIDs: []string{first.ID, second.ID},
		CreatedBy: "Taylor",
		Note:      "same root cause",
	})
	if err != nil {
		t.Fatalf("combine issues: %v", err)
	}
	if len(result.CreatedIssues) != 1 {
		t.Fatalf("expected 1 combined issue, got %d", len(result.CreatedIssues))
	}

	assertPublishedAndPersistedEvent(t, fixture.store, fixture.bus, 0, "combine", result.CreatedIssues[0].ID)
}

func TestSplitIssuePublishesSplitEventAfterCommit(t *testing.T) {
	t.Parallel()

	fixture := newCommandTestFixture()
	source := seedIssue(t, fixture.store, "Large umbrella issue", "Casey")
	handler := SplitIssueHandler{
		Runner:   fixture.runner,
		Enricher: fixture.enricher,
		Events:   fixture.bus,
	}

	result, err := handler.Handle(context.Background(), SplitIssue{
		SourceID:  source.ID,
		CreatedBy: "Taylor",
		Note:      "break out by platform",
		Children: []SplitIssueChild{
			{Raw: "iOS-specific follow-up"},
			{Raw: "macOS-specific follow-up"},
		},
	})
	if err != nil {
		t.Fatalf("split issue: %v", err)
	}
	if len(result.CreatedIssues) != 2 {
		t.Fatalf("expected 2 created issues, got %d", len(result.CreatedIssues))
	}

	assertPublishedAndPersistedEvent(t, fixture.store, fixture.bus, 0, "split", source.ID)
}
