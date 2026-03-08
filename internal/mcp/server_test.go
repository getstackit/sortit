package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcptypes "github.com/mark3labs/mcp-go/mcp"

	"splat/internal/issues"
	issuemap "splat/internal/map"
)

func TestHandleCreateIssue(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC)
	expected := issues.Issue{
		ID:        "issue-000003",
		Raw:       "Safari crashes when exporting a PDF",
		Tags:      []string{"bug", "safari", "export"},
		CreatedBy: "Casey",
		CreatedAt: createdAt,
		Status:    issues.StatusOpen,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/issues" {
			t.Fatalf("expected /issues path, got %s", r.URL.Path)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload["raw"] != expected.Raw {
			t.Fatalf("expected raw %q, got %q", expected.Raw, payload["raw"])
		}
		if payload["createdBy"] != expected.CreatedBy {
			t.Fatalf("expected createdBy %q, got %q", expected.CreatedBy, payload["createdBy"])
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expected); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	handler := &handlers{
		baseURL: server.URL,
		client:  server.Client(),
	}

	result, err := handler.handleCreateIssue(context.Background(), toolRequest(map[string]any{
		"raw":        expected.Raw,
		"created_by": expected.CreatedBy,
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
	if created.ID != expected.ID {
		t.Fatalf("expected ID %q, got %q", expected.ID, created.ID)
	}
	if firstText(result) == "" {
		t.Fatal("expected text fallback in MCP result")
	}
}

func TestHandleGetIssue(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC)
	expected := issues.Issue{
		ID:        "issue-000003",
		Raw:       "Safari crashes when exporting a PDF",
		Tags:      []string{"bug", "safari", "export"},
		CreatedBy: "Casey",
		CreatedAt: createdAt,
		Status:    issues.StatusOpen,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/issues/issue-000003" {
			t.Fatalf("expected /issues/issue-000003 path, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expected); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	handler := &handlers{
		baseURL: server.URL,
		client:  server.Client(),
	}

	result, err := handler.handleGetIssue(context.Background(), toolRequest(map[string]any{
		"id": " issue-000003 ",
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
	if issue.ID != expected.ID {
		t.Fatalf("expected ID %q, got %q", expected.ID, issue.ID)
	}
	if firstText(result) == "" {
		t.Fatal("expected text fallback in MCP result")
	}
}

func TestHandleGetIssueNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/issues/issue-000999" {
			t.Fatalf("expected /issues/issue-000999 path, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "issue not found"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	handler := &handlers{
		baseURL: server.URL,
		client:  server.Client(),
	}

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

func TestHandleRefineIssue(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC)
	refinedAt := createdAt.Add(15 * time.Minute)
	expected := issues.Issue{
		ID:        "issue-000003",
		Raw:       "Safari crashes when exporting a PDF after tapping share twice.",
		Tags:      []string{"bug", "safari", "export"},
		CreatedBy: "Casey",
		CreatedAt: createdAt,
		Status:    issues.StatusOpen,
		Discussion: []issues.IssuePost{
			{
				ID:        "issue-000003-post-000001",
				IssueID:   "issue-000003",
				Raw:       "Safari crashes when exporting a PDF",
				CreatedBy: "Casey",
				CreatedAt: createdAt,
				Sequence:  1,
			},
			{
				ID:        "issue-000003-post-000002",
				IssueID:   "issue-000003",
				Raw:       "Happens after tapping share twice on iPad.",
				CreatedBy: "Jordan",
				CreatedAt: refinedAt,
				Sequence:  2,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/issues/issue-000003/refine" {
			t.Fatalf("expected /issues/issue-000003/refine path, got %s", r.URL.Path)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload["raw"] != expected.Discussion[1].Raw {
			t.Fatalf("expected raw %q, got %q", expected.Discussion[1].Raw, payload["raw"])
		}
		if payload["createdBy"] != expected.Discussion[1].CreatedBy {
			t.Fatalf("expected createdBy %q, got %q", expected.Discussion[1].CreatedBy, payload["createdBy"])
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expected); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	handler := &handlers{
		baseURL: server.URL,
		client:  server.Client(),
	}

	result, err := handler.handleRefineIssue(context.Background(), toolRequest(map[string]any{
		"id":         " issue-000003 ",
		"raw":        expected.Discussion[1].Raw,
		"created_by": expected.Discussion[1].CreatedBy,
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
}

func TestHandleCloseIssue(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC)
	closedAt := createdAt.Add(2 * time.Hour)
	expected := issues.Issue{
		ID:        "issue-000003",
		Raw:       "Safari crashes when exporting a PDF",
		Tags:      []string{"bug", "safari", "export"},
		CreatedBy: "Casey",
		CreatedAt: createdAt,
		Status:    issues.StatusClosed,
		ClosedAt:  &closedAt,
		ClosedBy:  "Jordan",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/issues/issue-000003/close" {
			t.Fatalf("expected /issues/issue-000003/close path, got %s", r.URL.Path)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload["closedBy"] != expected.ClosedBy {
			t.Fatalf("expected closedBy %q, got %q", expected.ClosedBy, payload["closedBy"])
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expected); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	handler := &handlers{
		baseURL: server.URL,
		client:  server.Client(),
	}

	result, err := handler.handleCloseIssue(context.Background(), toolRequest(map[string]any{
		"id":        " issue-000003 ",
		"closed_by": expected.ClosedBy,
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
	if issue.ClosedBy != expected.ClosedBy {
		t.Fatalf("expected ClosedBy %q, got %q", expected.ClosedBy, issue.ClosedBy)
	}
}

func TestHandleCloseIssueNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/issues/issue-000999/close" {
			t.Fatalf("expected /issues/issue-000999/close path, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "issue not found"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	handler := &handlers{
		baseURL: server.URL,
		client:  server.Client(),
	}

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

	expected := issuemap.ExploreResponse{
		Issue: issuemap.ExploreIssue{
			ID:     "issue-000003",
			Raw:    "Safari crashes when exporting a PDF",
			Status: issues.StatusOpen,
			Tags: []issuemap.TagRelevance{
				{Tag: "export", Relevance: 0.92},
				{Tag: "safari", Relevance: 0.9},
			},
		},
		RelatedIssues: []issuemap.RelatedIssue{
			{
				ID:                 "issue-000004",
				Raw:                "PDF export fails in Safari",
				Status:             issues.StatusOpen,
				Tags:               []issuemap.TagRelevance{{Tag: "export", Relevance: 0.9}},
				SemanticSimilarity: 0.94,
				FactorSimilarity:   0.89,
				CombinedSimilarity: 0.92,
				Reason:             "Shared factor relevance in export, safari",
			},
		},
		Opportunities: []issuemap.Opportunity{
			{
				Title:    "Solve export + safari issues together",
				Summary:  "A single fix may address issue-000003 and issue-000004.",
				Theme:    "export + safari",
				IssueIDs: []string{"issue-000003", "issue-000004"},
				Issues: []issuemap.OpportunityIssue{
					{
						ID:          "issue-000003",
						Description: "Safari crashes when exporting a PDF",
						Status:      issues.StatusOpen,
					},
					{
						ID:          "issue-000004",
						Description: "PDF export fails in Safari",
						Status:      issues.StatusOpen,
					},
				},
				SharedTags: []string{"export", "safari"},
				Confidence: 0.92,
				Reason:     "These issues share strong factor relevance in export, safari.",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/issues/issue-000003/explore" {
			t.Fatalf("expected /issues/issue-000003/explore path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Fatalf("expected limit query 5, got %q", r.URL.Query().Get("limit"))
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expected); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	handler := &handlers{
		baseURL: server.URL,
		client:  server.Client(),
	}

	result, err := handler.handleExploreIssue(context.Background(), toolRequest(map[string]any{
		"id":    " issue-000003 ",
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
	if response.Issue.ID != expected.Issue.ID {
		t.Fatalf("expected target ID %q, got %q", expected.Issue.ID, response.Issue.ID)
	}
	if len(response.RelatedIssues) != 1 {
		t.Fatalf("expected 1 related issue, got %d", len(response.RelatedIssues))
	}
	if len(response.Opportunities) != 1 || len(response.Opportunities[0].Issues) != 2 {
		t.Fatalf("expected opportunity issues descriptions, got %+v", response.Opportunities)
	}
	if response.Opportunities[0].Issues[0].Description != expected.Issue.Raw {
		t.Fatalf("expected target description %q, got %q", expected.Issue.Raw, response.Opportunities[0].Issues[0].Description)
	}
	if firstText(result) == "" {
		t.Fatal("expected text fallback in MCP result")
	}
}

func TestHandleExploreIssueRejectsInvalidLimit(t *testing.T) {
	t.Parallel()

	handler := &handlers{}

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
