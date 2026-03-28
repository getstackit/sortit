package mcp

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"splat/internal/ai"
	"splat/internal/auth"
	"splat/internal/commands"
	issueenrichment "splat/internal/issueenrichment"
	"splat/internal/issueevents"
	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/queries"
	"splat/internal/services"
)

func TestHandleCreateIssue(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()

	result, err := handler.handleCreateIssue(context.Background(), toolRequest(map[string]any{
		"raw":        "Safari crashes when exporting a PDF",
		"created_by": "Casey",
	}))
	if err != nil {
		t.Fatalf("handleCreateIssue returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	created, ok := result.StructuredContent.(issues.Issue)
	if !ok {
		t.Fatalf("expected issues.Issue structured content, got %T", result.StructuredContent)
	}
	if created.ID == "" {
		t.Fatal("expected created issue ID")
	}
	if created.CreatedBy != "Casey" {
		t.Fatalf("expected CreatedBy Casey, got %q", created.CreatedBy)
	}
	if firstText(result) == "" {
		t.Fatal("expected text fallback in MCP result")
	}
}

func TestHandleCreateIssueUsesAuthenticatedPrincipal(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID:      "user-123",
		Login:       "casey",
		DisplayName: "Casey Authenticated",
	})

	result, err := handler.handleCreateIssue(ctx, toolRequest(map[string]any{
		"raw":        "Safari crashes when exporting a PDF",
		"created_by": "Ignored",
	}))
	if err != nil {
		t.Fatalf("handleCreateIssue returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	created := result.StructuredContent.(issues.Issue)
	if created.CreatedBy != "Casey Authenticated" {
		t.Fatalf("expected authenticated actor, got %q", created.CreatedBy)
	}
}

func TestHandleGetIssue(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	created := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")

	result, err := handler.handleGetIssue(context.Background(), toolRequest(map[string]any{
		"id": " " + created.ID + " ",
	}))
	if err != nil {
		t.Fatalf("handleGetIssue returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	issue, ok := result.StructuredContent.(issues.Issue)
	if !ok {
		t.Fatalf("expected issues.Issue structured content, got %T", result.StructuredContent)
	}
	if issue.ID != created.ID {
		t.Fatalf("expected ID %q, got %q", created.ID, issue.ID)
	}
}

func TestHandleGetIssueNotFound(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()

	result, err := handler.handleGetIssue(context.Background(), toolRequest(map[string]any{
		"id": "issue-000999",
	}))
	if err != nil {
		t.Fatalf("handleGetIssue returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result")
	}
	if !strings.Contains(firstText(result), "issue not found") {
		t.Fatalf("expected issue not found error, got %q", firstText(result))
	}
}

func TestHandleListTags(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")

	result, err := handler.handleListTags(context.Background(), toolRequest(nil))
	if err != nil {
		t.Fatalf("handleListTags returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	tags, ok := result.StructuredContent.([]issues.Tag)
	if !ok {
		t.Fatalf("expected []issues.Tag structured content, got %T", result.StructuredContent)
	}
	if len(tags) == 0 {
		t.Fatal("expected stored tags")
	}
	if tags[0].Name == "" {
		t.Fatalf("expected tag name, got %#v", tags[0])
	}
	if firstText(result) == "" {
		t.Fatal("expected text fallback in MCP result")
	}
}

func TestHandleSearchIssues(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")
	createTestIssue(t, handler, "PDF export fails in Safari after tapping share twice", "Jordan")

	result, err := handler.handleSearchIssues(context.Background(), toolRequest(map[string]any{
		"query":  "safari pdf export",
		"limit":  3,
		"status": "all",
	}))
	if err != nil {
		t.Fatalf("handleSearchIssues returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	response, ok := result.StructuredContent.(issuemap.SearchResponse)
	if !ok {
		t.Fatalf("expected issuemap.SearchResponse structured content, got %T", result.StructuredContent)
	}
	if len(response.RelatedIssues) == 0 {
		t.Fatal("expected related issues in search response")
	}
	if firstText(result) == "" {
		t.Fatal("expected text fallback in MCP result")
	}
}

func TestHandleSearchIssuesRejectsInvalidStatus(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	result, err := handler.handleSearchIssues(context.Background(), toolRequest(map[string]any{
		"query":  "front end changes",
		"status": "recent",
	}))
	if err != nil {
		t.Fatalf("handleSearchIssues returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result")
	}
	if !strings.Contains(firstText(result), "status must be one of") {
		t.Fatalf("expected status validation error, got %q", firstText(result))
	}
}

func TestHandleRefineIssues(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	created := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")

	result, err := handler.handleRefineIssues(context.Background(), toolRequest(map[string]any{
		"ids":        []string{" " + created.ID + " "},
		"raw":        "Happens after tapping share twice on iPad.",
		"created_by": "Jordan",
	}))
	if err != nil {
		t.Fatalf("handleRefineIssues returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	response, ok := result.StructuredContent.(commands.IssueMutationResult)
	if !ok {
		t.Fatalf("expected commands.IssueMutationResult structured content, got %T", result.StructuredContent)
	}
	if response.Summary.Succeeded != 1 || response.Summary.Failed != 0 {
		t.Fatalf("unexpected summary: %#v", response.Summary)
	}
	if len(response.Results) != 1 || response.Results[0].Issue == nil {
		t.Fatalf("expected per-issue success result, got %#v", response.Results)
	}
	if len(response.Results[0].Issue.Discussion) != 2 {
		t.Fatalf("expected discussion in refined issue, got %#v", response.Results[0].Issue.Discussion)
	}
	if response.Results[0].Issue.Discussion[1].CreatedBy != "Jordan" {
		t.Fatalf("expected refinement author Jordan, got %q", response.Results[0].Issue.Discussion[1].CreatedBy)
	}
}

func TestHandleRefineIssuesUsesAuthenticatedPrincipal(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	created := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID:      "user-123",
		Login:       "casey",
		DisplayName: "Casey Authenticated",
	})

	result, err := handler.handleRefineIssues(ctx, toolRequest(map[string]any{
		"ids":        []string{created.ID},
		"raw":        "Happens after tapping share twice on iPad.",
		"created_by": "Ignored",
	}))
	if err != nil {
		t.Fatalf("handleRefineIssues returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	response := result.StructuredContent.(commands.IssueMutationResult)
	if response.Results[0].Issue.Discussion[1].CreatedBy != "Casey Authenticated" {
		t.Fatalf("expected authenticated actor, got %q", response.Results[0].Issue.Discussion[1].CreatedBy)
	}
}

func TestHandleProgressIssues(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	created := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")

	result, err := handler.handleProgressIssues(context.Background(), toolRequest(map[string]any{
		"ids":        []string{" " + created.ID + " "},
		"raw":        "Identified the root cause in the share handler.",
		"created_by": "Jordan",
	}))
	if err != nil {
		t.Fatalf("handleProgressIssues returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	response, ok := result.StructuredContent.(commands.IssueMutationResult)
	if !ok {
		t.Fatalf("expected commands.IssueMutationResult structured content, got %T", result.StructuredContent)
	}
	if len(response.Results) != 1 || response.Results[0].Issue == nil {
		t.Fatalf("expected per-issue success result, got %#v", response.Results)
	}
	if len(response.Results[0].Issue.Discussion) != 2 {
		t.Fatalf("expected discussion in progress issue, got %#v", response.Results[0].Issue.Discussion)
	}
	if response.Results[0].Issue.Discussion[1].Kind != "progress" {
		t.Fatalf("expected progress post kind, got %q", response.Results[0].Issue.Discussion[1].Kind)
	}
}

func TestHandleCloseIssues(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	created := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")

	result, err := handler.handleCloseIssues(context.Background(), toolRequest(map[string]any{
		"ids":       []string{" " + created.ID + " "},
		"closed_by": "Jordan",
	}))
	if err != nil {
		t.Fatalf("handleCloseIssues returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	response, ok := result.StructuredContent.(commands.IssueMutationResult)
	if !ok {
		t.Fatalf("expected commands.IssueMutationResult structured content, got %T", result.StructuredContent)
	}
	if len(response.Results) != 1 || response.Results[0].Issue == nil {
		t.Fatalf("expected per-issue success result, got %#v", response.Results)
	}
	if response.Results[0].Issue.Status != issues.StatusClosed {
		t.Fatalf("expected closed issue, got %q", response.Results[0].Issue.Status)
	}
	if response.Results[0].Issue.ClosedBy != "Jordan" {
		t.Fatalf("expected ClosedBy Jordan, got %q", response.Results[0].Issue.ClosedBy)
	}
}

func TestHandleCloseIssuesSupportsPartialFailure(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	created := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")

	result, err := handler.handleCloseIssues(context.Background(), toolRequest(map[string]any{
		"ids": []string{created.ID, "issue-000999", created.ID},
	}))
	if err != nil {
		t.Fatalf("handleCloseIssues returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	response := result.StructuredContent.(commands.IssueMutationResult)
	if got := response.RequestedIDs; len(got) != 2 || got[0] != created.ID || got[1] != "issue-000999" {
		t.Fatalf("expected sanitized requested ids, got %#v", got)
	}
	if response.Summary.Requested != 2 || response.Summary.Succeeded != 1 || response.Summary.Failed != 1 {
		t.Fatalf("unexpected summary: %#v", response.Summary)
	}
	if len(response.Failed) != 1 || response.Failed[0].Error != "issue not found" {
		t.Fatalf("expected issue not found failure, got %#v", response.Failed)
	}
}

func TestHandleAssignIssuesAllowsUnassign(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	created := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")
	_, err := handler.assignIssues.Handle(context.Background(), commands.AssignIssues{
		IDs:        []string{created.ID},
		AssignedTo: "Jordan",
	})
	if err != nil {
		t.Fatalf("seed assign: %v", err)
	}

	result, err := handler.handleAssignIssues(context.Background(), toolRequest(map[string]any{
		"ids":         []string{created.ID},
		"assigned_to": "   ",
	}))
	if err != nil {
		t.Fatalf("handleAssignIssues returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	response := result.StructuredContent.(commands.IssueMutationResult)
	if response.Results[0].Issue.AssignedTo != "" {
		t.Fatalf("expected issue to be unassigned, got %q", response.Results[0].Issue.AssignedTo)
	}
}

func TestHandleAssignIssuesRequiresIDs(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()

	result, err := handler.handleAssignIssues(context.Background(), toolRequest(map[string]any{
		"ids":         []string{" ", " "},
		"assigned_to": "Jordan",
	}))
	if err != nil {
		t.Fatalf("handleAssignIssues returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result")
	}
	if !strings.Contains(firstText(result), "at least one issue id is required") {
		t.Fatalf("expected ids validation error, got %q", firstText(result))
	}
}

func TestHandleExploreIssue(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	target := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")
	createTestIssue(t, handler, "PDF export fails in Safari after tapping share twice", "Jordan")

	result, err := handler.handleExploreIssue(context.Background(), toolRequest(map[string]any{
		"id":    " " + target.ID + " ",
		"limit": 5,
	}))
	if err != nil {
		t.Fatalf("handleExploreIssue returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	response, ok := result.StructuredContent.(issuemap.ExploreResponse)
	if !ok {
		t.Fatalf("expected ExploreResponse structured content, got %T", result.StructuredContent)
	}
	if response.Issue.ID != target.ID {
		t.Fatalf("expected target ID %q, got %q", target.ID, response.Issue.ID)
	}
	if len(response.RelatedIssues) == 0 {
		t.Fatal("expected related issues in explore response")
	}
	if len(response.Opportunities) == 0 {
		t.Fatal("expected explore opportunities")
	}
	if firstText(result) == "" {
		t.Fatal("expected text fallback in MCP result")
	}
}

func TestHandleExploreIssueRejectsInvalidLimit(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()

	result, err := handler.handleExploreIssue(context.Background(), toolRequest(map[string]any{
		"id":    "issue-000003",
		"limit": 0,
	}))
	if err != nil {
		t.Fatalf("handleExploreIssue returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result")
	}
	if !strings.Contains(firstText(result), "limit must be greater than 0") {
		t.Fatalf("expected invalid limit error, got %q", firstText(result))
	}
}

func newTestHandlers() *handlers {
	analyzer := ai.NewAnalyzerWithCanonicalizer(
		ai.NewStubTagger(),
		ai.NewStubEmbedder(),
		ai.NewStubCanonicalizer(),
	)
	store := issues.NewInMemoryStore(nil)
	catalog := services.NewCatalogService(nil, analyzer, slog.Default())
	enricher := issueenrichment.NewIssueEnricher(analyzer, catalog, slog.Default())
	runner := &commands.CommandRunner{DB: store}
	eventBus := issueevents.NewEventBus()

	return &handlers{
		createIssue: commands.CreateIssueHandler{
			Logger:   slog.Default(),
			Runner:   runner,
			Enricher: enricher,
			Events:   eventBus,
		},
		refineIssues:     commands.RefineIssuesHandler{RefineIssue: commands.RefineIssueHandler{Runner: runner, Store: store, Enricher: enricher, Events: eventBus}},
		progressIssues:   commands.ProgressIssuesHandler{ProgressIssue: commands.ProgressIssueHandler{Runner: runner, Events: eventBus}},
		closeIssues:      commands.CloseIssuesHandler{CloseIssue: commands.CloseIssueHandler{Runner: runner, Events: eventBus}},
		assignIssues:     commands.AssignIssuesHandler{AssignIssue: commands.AssignIssueHandler{Runner: runner, Events: eventBus}},
		splitIssue:       commands.SplitIssueHandler{Runner: runner, Enricher: enricher, Events: eventBus},
		combineIssues:    commands.CombineIssuesHandler{Runner: runner, Store: store, Enricher: enricher, Events: eventBus},
		linkIssues:       commands.LinkIssuesHandler{Runner: runner, Events: eventBus},
		getIssue:         queries.GetIssueHandler{Store: store},
		listTags:         queries.ListTagsHandler{Catalog: catalog},
		searchIssues:     queries.SearchIssuesHandler{Analyzer: analyzer, Catalog: catalog, Store: store},
		exploreIssue:     queries.ExploreIssueHandler{Reader: store, DetailReader: store, Catalog: catalog},
		getPersonProfile: queries.GetPersonProfileHandler{Store: store},
		workCorrelations: queries.WorkCorrelationsHandler{Store: store},
	}
}

func createTestIssue(t *testing.T, handler *handlers, raw string, createdBy string) issues.Issue {
	t.Helper()

	issue, err := handler.createIssue.Handle(context.Background(), commands.CreateIssue{
		Raw:       raw,
		CreatedBy: createdBy,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	drainTestEnrichment(t, handler)
	return issue
}

func drainTestEnrichment(t *testing.T, handler *handlers) {
	t.Helper()

	claimer, ok := handler.getIssue.Store.(issues.EnrichmentJobClaimer)
	if !ok {
		return
	}
	store, ok := handler.getIssue.Store.(issues.Store)
	if !ok {
		t.Fatal("expected get issue reader to also implement issues.Store in tests")
	}
	worker := &issueenrichment.IssueEnrichmentWorker{
		Logger:   slog.Default(),
		Store:    store,
		DB:       handler.createIssue.Runner.DB,
		Jobs:     claimer,
		Enricher: handler.createIssue.Enricher,
	}
	for {
		processed, err := worker.ProcessOne(context.Background())
		if err != nil {
			t.Fatalf("process test enrichment: %v", err)
		}
		if !processed {
			return
		}
	}
}

func toolRequest(arguments map[string]any) mcptypes.CallToolRequest {
	return mcptypes.CallToolRequest{
		Params: mcptypes.CallToolParams{
			Arguments: arguments,
		},
	}
}

func firstText(result *mcptypes.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}

	text, ok := result.Content[0].(mcptypes.TextContent)
	if !ok {
		return ""
	}

	return text.Text
}
