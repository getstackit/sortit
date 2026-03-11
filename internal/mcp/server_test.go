package mcp

import (
	"context"
	"strings"
	"testing"

	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"splat/internal/ai"
	"splat/internal/auth"
	"splat/internal/commands"
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

func TestHandleRefineIssue(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	created := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")

	result, err := handler.handleRefineIssue(context.Background(), toolRequest(map[string]any{
		"id":         " " + created.ID + " ",
		"raw":        "Happens after tapping share twice on iPad.",
		"created_by": "Jordan",
	}))
	if err != nil {
		t.Fatalf("handleRefineIssue returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	issue, ok := result.StructuredContent.(issues.Issue)
	if !ok {
		t.Fatalf("expected issues.Issue structured content, got %T", result.StructuredContent)
	}
	if len(issue.Discussion) != 2 {
		t.Fatalf("expected discussion in refined issue, got %#v", issue.Discussion)
	}
	if issue.Discussion[1].CreatedBy != "Jordan" {
		t.Fatalf("expected refinement author Jordan, got %q", issue.Discussion[1].CreatedBy)
	}
}

func TestHandleProgressIssue(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	created := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")

	result, err := handler.handleProgressIssue(context.Background(), toolRequest(map[string]any{
		"id":         " " + created.ID + " ",
		"raw":        "Identified the root cause in the share handler.",
		"created_by": "Jordan",
	}))
	if err != nil {
		t.Fatalf("handleProgressIssue returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	issue, ok := result.StructuredContent.(issues.Issue)
	if !ok {
		t.Fatalf("expected issues.Issue structured content, got %T", result.StructuredContent)
	}
	if len(issue.Discussion) != 2 {
		t.Fatalf("expected discussion in progress issue, got %#v", issue.Discussion)
	}
	if issue.Discussion[1].Kind != "progress" {
		t.Fatalf("expected progress post kind, got %q", issue.Discussion[1].Kind)
	}
}

func TestHandleCloseIssue(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()
	created := createTestIssue(t, handler, "Safari crashes when exporting a PDF", "Casey")

	result, err := handler.handleCloseIssue(context.Background(), toolRequest(map[string]any{
		"id":        " " + created.ID + " ",
		"closed_by": "Jordan",
	}))
	if err != nil {
		t.Fatalf("handleCloseIssue returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", firstText(result))
	}

	issue, ok := result.StructuredContent.(issues.Issue)
	if !ok {
		t.Fatalf("expected issues.Issue structured content, got %T", result.StructuredContent)
	}
	if issue.Status != issues.StatusClosed {
		t.Fatalf("expected closed issue, got %q", issue.Status)
	}
	if issue.ClosedBy != "Jordan" {
		t.Fatalf("expected ClosedBy Jordan, got %q", issue.ClosedBy)
	}
}

func TestHandleCloseIssueNotFound(t *testing.T) {
	t.Parallel()

	handler := newTestHandlers()

	result, err := handler.handleCloseIssue(context.Background(), toolRequest(map[string]any{
		"id": "issue-000999",
	}))
	if err != nil {
		t.Fatalf("handleCloseIssue returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result")
	}
	if !strings.Contains(firstText(result), "issue not found") {
		t.Fatalf("expected issue not found error, got %q", firstText(result))
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
	catalog := services.NewCatalogService(nil, analyzer)
	enricher := services.NewIssueEnricher(analyzer, catalog)
	runner := &commands.CommandRunner{DB: store}
	eventBus := issues.NewEventBus()

	return &handlers{
		createIssue: commands.CreateIssueHandler{
			Runner:   runner,
			Enricher: enricher,
			Events:   eventBus,
		},
		refineIssue:      commands.RefineIssueHandler{Runner: runner, Store: store, Enricher: enricher, Events: eventBus},
		progressIssue:    commands.ProgressIssueHandler{Runner: runner, Events: eventBus},
		closeIssue:       commands.CloseIssueHandler{Runner: runner, Events: eventBus},
		assignIssue:      commands.AssignIssueHandler{Runner: runner, Events: eventBus},
		splitIssue:       commands.SplitIssueHandler{Runner: runner, Enricher: enricher, Events: eventBus},
		combineIssues:    commands.CombineIssuesHandler{Runner: runner, Store: store, Enricher: enricher, Events: eventBus},
		linkIssues:       commands.LinkIssuesHandler{Runner: runner, Events: eventBus},
		getIssue:         queries.GetIssueHandler{Store: store},
		searchIssues:     queries.SearchIssuesHandler{Analyzer: analyzer, Catalog: catalog, Store: store},
		exploreIssue:     queries.ExploreIssueHandler{Store: store, Catalog: catalog},
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
	return issue
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
