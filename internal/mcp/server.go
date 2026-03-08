package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type ServerConfig struct {
	// BaseURL is the Splat API base URL, e.g. "http://localhost:8081/api/v1"
	BaseURL string
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

	httpServer := server.NewStreamableHTTPServer(s)
	return httpServer
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

func (h *handlers) doJSONRequest(ctx context.Context, method, route string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		requestBody, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to encode request: %v", err)
		}
		body = bytes.NewReader(requestBody)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, h.baseURL+route, body)
	if err != nil {
		return fmt.Errorf("failed to build request: %v", err)
	}
	if payload != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to reach Splat API: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode >= 400 {
		return errors.New(formatAPIError(resp.StatusCode, respBody))
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}

	return nil
}

func formatAPIError(statusCode int, respBody []byte) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &payload); err == nil {
		payload.Error = strings.TrimSpace(payload.Error)
		if payload.Error != "" {
			return fmt.Sprintf("Splat API error (%d): %s", statusCode, payload.Error)
		}
	}

	message := strings.TrimSpace(string(respBody))
	if message == "" {
		message = http.StatusText(statusCode)
	}

	return fmt.Sprintf("Splat API error (%d): %s", statusCode, message)
}
