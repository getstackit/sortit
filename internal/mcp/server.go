package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

	body, _ := json.Marshal(map[string]string{
		"raw":       raw,
		"createdBy": createdBy,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+"/issues", bytes.NewReader(body))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to build request: %v", err)), nil
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to reach Splat API: %v", err)), nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read response: %v", err)), nil
	}

	if resp.StatusCode >= 400 {
		return mcp.NewToolResultError(fmt.Sprintf("Splat API error (%d): %s", resp.StatusCode, string(respBody))), nil
	}

	return mcp.NewToolResultText(string(respBody)), nil
}
