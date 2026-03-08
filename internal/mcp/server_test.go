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
