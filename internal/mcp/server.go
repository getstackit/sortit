package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"splat/internal/issues"
	issuemap "splat/internal/map"
)

type authorizationContextKey struct{}

type ServerConfig struct {
	// BaseURL is the Splat API base URL, e.g. "http://localhost:8081/api/v1"
	BaseURL string
}

type apiError struct {
	statusCode int
	message    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("Splat API error (%d): %s", e.statusCode, e.message)
}

// NewHandler creates a Streamable HTTP handler for the MCP server,
// suitable for mounting on an existing HTTP mux at a path like "/mcp".
func NewHandler(cfg ServerConfig) http.Handler {
	s := server.NewMCPServer(
		"splat",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	h := &handlers{
		baseURL: cfg.BaseURL,
		client:  &http.Client{},
	}

	s.AddTool(
		mcp.NewTool("create_issue",
			mcp.WithDescription("Create a new issue in Splat. Submit raw text (bug report, feature idea, stack trace, customer quote, etc.) and Splat will automatically tag and categorize it using AI."),
			mcp.WithString("raw",
				mcp.Required(),
				mcp.Description("The raw issue text. Can be a bug report, feature request, idea, stack trace, customer quote, or any freeform text."),
			),
			mcp.WithString("created_by",
				mcp.Description("Who created this issue. Defaults to 'Claude'."),
			),
		),
		h.handleCreateIssue,
	)

	s.AddTool(
		mcp.NewTool("search_issues",
			mcp.WithDescription("Search stored Splat issues from arbitrary text. Returns the most related issues using a blend of semantic similarity and tag-factor relevance."),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("The search text, page description, or other freeform query to match against stored issues."),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of related issues to return. Defaults to 8."),
			),
			mcp.WithString("status",
				mcp.Description("Optional issue status filter: open, closed, or all. Defaults to open."),
			),
		),
		h.handleSearchIssues,
	)

	s.AddTool(
		mcp.NewTool("get_issue",
			mcp.WithDescription("Get a Splat issue by ID."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("The issue ID, for example issue-000003."),
			),
		),
		h.handleGetIssue,
	)

	s.AddTool(
		mcp.NewTool("refine_issue",
			mcp.WithDescription("Refine an existing Splat issue by appending discussion context or feedback. This updates the issue's canonical description, tags, and semantic similarity."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("The issue ID to refine, for example issue-000003."),
			),
			mcp.WithString("raw",
				mcp.Required(),
				mcp.Description("The new discussion post to append as additional context, refinement, or feedback."),
			),
			mcp.WithString("created_by",
				mcp.Description("Who authored the refinement. Defaults to 'Claude'."),
			),
		),
		h.handleRefineIssue,
	)

	s.AddTool(
		mcp.NewTool("progress_issue",
			mcp.WithDescription("Post a progress update on an existing Splat issue. Progress updates report work done toward resolving an issue without changing the canonical summary, tags, or semantic similarity."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("The issue ID to post progress on, for example issue-000003."),
			),
			mcp.WithString("raw",
				mcp.Required(),
				mcp.Description("The progress update text describing work done toward resolving the issue."),
			),
			mcp.WithString("created_by",
				mcp.Description("Who authored the progress update. Defaults to 'Claude'."),
			),
		),
		h.handleProgressIssue,
	)

	s.AddTool(
		mcp.NewTool("close_issue",
			mcp.WithDescription("Close a Splat issue by ID."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("The issue ID to close, for example issue-000003."),
			),
			mcp.WithString("closed_by",
				mcp.Description("Who closed the issue. Defaults to 'Claude'."),
			),
		),
		h.handleCloseIssue,
	)

	s.AddTool(
		mcp.NewTool("assign_issue",
			mcp.WithDescription("Assign a Splat issue to a person, or unassign by passing an empty string."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("The issue ID to assign, for example issue-000003."),
			),
			mcp.WithString("assigned_to",
				mcp.Required(),
				mcp.Description("The person to assign the issue to. Pass an empty string to unassign."),
			),
		),
		h.handleAssignIssue,
	)

	s.AddTool(
		mcp.NewTool("split_issue",
			mcp.WithDescription("Split one issue into multiple child issues. This creates child issues, records parent/child relationships, and can optionally close the source issue."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("The source issue ID to split."),
			),
			mcp.WithArray("children_raw",
				mcp.Required(),
				mcp.Description("The raw text for each child issue to create."),
				mcp.WithStringItems(),
			),
			mcp.WithBoolean("close_source",
				mcp.Description("Whether to automatically close the source issue after splitting."),
			),
			mcp.WithString("note",
				mcp.Description("Optional rationale to attach to the grouped split operation."),
			),
			mcp.WithString("created_by",
				mcp.Description("Who performed the split. Defaults to 'Claude'."),
			),
		),
		h.handleSplitIssue,
	)

	s.AddTool(
		mcp.NewTool("combine_issues",
			mcp.WithDescription("Combine multiple source issues into a new canonical issue. The new issue text is synthesized automatically and the source issues are closed."),
			mcp.WithArray("ids",
				mcp.Required(),
				mcp.Description("The source issue IDs to combine."),
				mcp.WithStringItems(),
			),
			mcp.WithString("note",
				mcp.Description("Optional rationale to attach to the grouped combine operation."),
			),
			mcp.WithString("created_by",
				mcp.Description("Who performed the combine. Defaults to 'Claude'."),
			),
		),
		h.handleCombineIssues,
	)

	s.AddTool(
		mcp.NewTool("link_issues",
			mcp.WithDescription("Create a direct relationship between two existing issues without creating a new issue."),
			mcp.WithString("source_id",
				mcp.Required(),
				mcp.Description("The source issue ID."),
			),
			mcp.WithString("target_id",
				mcp.Required(),
				mcp.Description("The target issue ID."),
			),
			mcp.WithString("type",
				mcp.Required(),
				mcp.Description("Relationship type: parent_of, child_of, merged_into, derived_from, related_to, or duplicate_of."),
			),
			mcp.WithString("note",
				mcp.Description("Optional rationale to attach to the link operation."),
			),
			mcp.WithString("created_by",
				mcp.Description("Who created the link. Defaults to 'Claude'."),
			),
		),
		h.handleLinkIssues,
	)

	s.AddTool(
		mcp.NewTool("explore_issue",
			mcp.WithDescription("Explore a stored Splat issue by ID. Returns related open issues using semantic similarity and factor relevance, plus structured opportunities to solve multiple issues together."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("The issue ID to explore, for example issue-000003."),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of related issues to return. Defaults to 8."),
			),
		),
		h.handleExploreIssue,
	)

	s.AddTool(
		mcp.NewTool("get_person_profile",
			mcp.WithDescription("Get a person's tag profile based on their assigned issues. Shows where they spend their time across issue factors/tags."),
			mcp.WithString("person",
				mcp.Required(),
				mcp.Description("The person's name (must match the assignedTo field on issues)."),
			),
			mcp.WithString("status",
				mcp.Description("Optional issue status filter: open, closed, or all. Defaults to all."),
			),
		),
		h.handleGetPersonProfile,
	)

	s.AddTool(
		mcp.NewTool("work_correlations",
			mcp.WithDescription("Find people working on semantically similar or tag-overlapping work. Returns pairs of people with similarity scores."),
			mcp.WithString("status",
				mcp.Description("Optional issue status filter: open, closed, or all. Defaults to all."),
			),
		),
		h.handleWorkCorrelations,
	)

	httpServer := server.NewStreamableHTTPServer(s)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), authorizationContextKey{}, strings.TrimSpace(r.Header.Get("Authorization")))
		httpServer.ServeHTTP(w, r.WithContext(ctx))
	})
}

type handlers struct {
	baseURL string
	client  *http.Client
}

func (h *handlers) handleCreateIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw, err := req.RequireString("raw")
	if err != nil {
		return mcp.NewToolResultError("raw is required"), nil
	}

	createdBy := req.GetString("created_by", "Claude")

	var issue issues.Issue
	err = h.doJSONRequest(ctx, http.MethodPost, "/issues", map[string]string{
		"raw":       raw,
		"createdBy": createdBy,
	}, &issue)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(issue)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleGetIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	var issue issues.Issue
	err = h.doJSONRequest(ctx, http.MethodGet, "/issues/"+url.PathEscape(id), nil, &issue)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(issue)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleSearchIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	limit := req.GetInt("limit", 8)
	if limit <= 0 {
		return mcp.NewToolResultError("limit must be greater than 0"), nil
	}

	status := strings.ToLower(strings.TrimSpace(req.GetString("status", "open")))
	switch status {
	case "", "open":
		status = "open"
	case "closed", "all":
	default:
		return mcp.NewToolResultError("status must be one of: open, closed, all"), nil
	}

	queryString := url.Values{}
	queryString.Set("q", query)
	if limit != 8 {
		queryString.Set("limit", strconv.Itoa(limit))
	}
	if status != "open" {
		queryString.Set("status", status)
	}

	var response issuemap.SearchResponse
	err = h.doJSONRequest(ctx, http.MethodGet, "/issues/search?"+queryString.Encode(), nil, &response)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(response)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleRefineIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}

	raw, err := req.RequireString("raw")
	if err != nil {
		return mcp.NewToolResultError("raw is required"), nil
	}

	id = strings.TrimSpace(id)
	raw = strings.TrimSpace(raw)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}
	if raw == "" {
		return mcp.NewToolResultError("raw is required"), nil
	}

	createdBy := req.GetString("created_by", "Claude")

	var issue issues.Issue
	err = h.doJSONRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(id)+"/refine", map[string]string{
		"raw":       raw,
		"createdBy": createdBy,
	}, &issue)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(issue)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleProgressIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}

	raw, err := req.RequireString("raw")
	if err != nil {
		return mcp.NewToolResultError("raw is required"), nil
	}

	id = strings.TrimSpace(id)
	raw = strings.TrimSpace(raw)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}
	if raw == "" {
		return mcp.NewToolResultError("raw is required"), nil
	}

	createdBy := req.GetString("created_by", "Claude")

	var issue issues.Issue
	err = h.doJSONRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(id)+"/progress", map[string]string{
		"raw":       raw,
		"createdBy": createdBy,
	}, &issue)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(issue)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleCloseIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	closedBy := req.GetString("closed_by", "Claude")

	var issue issues.Issue
	err = h.doJSONRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(id)+"/close", map[string]string{
		"closedBy": closedBy,
	}, &issue)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(issue)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleExploreIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	limit := req.GetInt("limit", 8)
	if limit <= 0 {
		return mcp.NewToolResultError("limit must be greater than 0"), nil
	}

	var query string
	if limit != 8 {
		query = "?limit=" + strconv.Itoa(limit)
	}

	var response issuemap.ExploreResponse
	err = h.doJSONRequest(ctx, http.MethodGet, "/issues/"+url.PathEscape(id)+"/explore"+query, nil, &response)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(response)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleAssignIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}

	assignedTo, err := req.RequireString("assigned_to")
	if err != nil {
		return mcp.NewToolResultError("assigned_to is required"), nil
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	var issue issues.Issue
	err = h.doJSONRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(id)+"/assign", map[string]string{
		"assignedTo": assignedTo,
	}, &issue)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(issue)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleSplitIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}
	children, err := req.RequireStringSlice("children_raw")
	if err != nil {
		return mcp.NewToolResultError("children_raw is required"), nil
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	normalizedChildren := make([]map[string]any, 0, len(children))
	for _, child := range children {
		child = strings.TrimSpace(child)
		if child == "" {
			continue
		}
		normalizedChildren = append(normalizedChildren, map[string]any{"raw": child})
	}
	if len(normalizedChildren) == 0 {
		return mcp.NewToolResultError("children_raw must include at least one non-empty child"), nil
	}

	var resultPayload issues.IssueOperationResult
	err = h.doJSONRequest(ctx, http.MethodPost, "/issues/"+url.PathEscape(id)+"/split", map[string]any{
		"children":    normalizedChildren,
		"closeSource": req.GetBool("close_source", false),
		"note":        req.GetString("note", ""),
		"createdBy":   req.GetString("created_by", "Claude"),
	}, &resultPayload)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(resultPayload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleCombineIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ids, err := req.RequireStringSlice("ids")
	if err != nil {
		return mcp.NewToolResultError("ids is required"), nil
	}
	if len(ids) < 2 {
		return mcp.NewToolResultError("at least two issue ids are required"), nil
	}

	var resultPayload issues.IssueOperationResult
	err = h.doJSONRequest(ctx, http.MethodPost, "/issues/combine", map[string]any{
		"ids":       ids,
		"note":      req.GetString("note", ""),
		"createdBy": req.GetString("created_by", "Claude"),
	}, &resultPayload)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(resultPayload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleLinkIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceID, err := req.RequireString("source_id")
	if err != nil {
		return mcp.NewToolResultError("source_id is required"), nil
	}
	targetID, err := req.RequireString("target_id")
	if err != nil {
		return mcp.NewToolResultError("target_id is required"), nil
	}
	linkType, err := req.RequireString("type")
	if err != nil {
		return mcp.NewToolResultError("type is required"), nil
	}

	var resultPayload issues.IssueOperationResult
	err = h.doJSONRequest(ctx, http.MethodPost, "/issues/link", map[string]any{
		"sourceId":  sourceID,
		"targetId":  targetID,
		"type":      linkType,
		"note":      req.GetString("note", ""),
		"createdBy": req.GetString("created_by", "Claude"),
	}, &resultPayload)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := mcp.NewToolResultJSON(resultPayload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func (h *handlers) handleGetPersonProfile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	person, err := req.RequireString("person")
	if err != nil {
		return mcp.NewToolResultError("person is required"), nil
	}

	person = strings.TrimSpace(person)
	if person == "" {
		return mcp.NewToolResultError("person is required"), nil
	}

	status := strings.ToLower(strings.TrimSpace(req.GetString("status", "all")))
	switch status {
	case "", "all":
		status = "all"
	case "open", "closed":
	default:
		return mcp.NewToolResultError("status must be one of: open, closed, all"), nil
	}

	queryString := url.Values{}
	if status != "all" {
		queryString.Set("status", status)
	}

	query := ""
	if len(queryString) > 0 {
		query = "?" + queryString.Encode()
	}

	var response json.RawMessage
	err = h.doJSONRequest(ctx, http.MethodGet, "/people/"+url.PathEscape(person)+"/profile"+query, nil, &response)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(string(response)), nil
}

func (h *handlers) handleWorkCorrelations(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	status := strings.ToLower(strings.TrimSpace(req.GetString("status", "all")))
	switch status {
	case "", "all":
		status = "all"
	case "open", "closed":
	default:
		return mcp.NewToolResultError("status must be one of: open, closed, all"), nil
	}

	queryString := url.Values{}
	if status != "all" {
		queryString.Set("status", status)
	}

	query := ""
	if len(queryString) > 0 {
		query = "?" + queryString.Encode()
	}

	var response json.RawMessage
	err := h.doJSONRequest(ctx, http.MethodGet, "/people/correlations"+query, nil, &response)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(string(response)), nil
}

func (h *handlers) doJSONRequest(ctx context.Context, method, route string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		requestBody, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		body = bytes.NewReader(requestBody)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, h.baseURL+route, body)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	if payload != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if authHeader, ok := ctx.Value(authorizationContextKey{}).(string); ok && authHeader != "" {
		httpReq.Header.Set("Authorization", authHeader)
	}

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to reach Splat API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return &apiError{
			statusCode: resp.StatusCode,
			message:    formatAPIErrorMessage(resp.StatusCode, respBody),
		}
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

func formatAPIErrorMessage(statusCode int, respBody []byte) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &payload); err == nil {
		payload.Error = strings.TrimSpace(payload.Error)
		if payload.Error != "" {
			return payload.Error
		}
	}

	message := strings.TrimSpace(string(respBody))
	if message == "" {
		message = http.StatusText(statusCode)
	}

	return message
}
