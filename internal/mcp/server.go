package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"sortit/internal/auth"
	"sortit/internal/issues"
	issuecmd "sortit/internal/issues/commands"
	issueviews "sortit/internal/issues/views"
	"sortit/internal/mapview"
	"sortit/internal/people"
	"sortit/internal/search"
)

type ServerConfig struct {
	CreateIssue      issuecmd.CreateIssueHandler
	RefineIssue      issuecmd.RefineIssueHandler
	ProgressIssue    issuecmd.ProgressIssueHandler
	CloseIssue       issuecmd.CloseIssueHandler
	AssignIssue      issuecmd.AssignIssueHandler
	SplitIssue       issuecmd.SplitIssueHandler
	CombineIssues    issuecmd.CombineIssuesHandler
	LinkIssues       issuecmd.LinkIssuesHandler
	GetIssue         issueviews.GetIssueHandler
	ListTags         issueviews.ListTagsHandler
	SearchIssues     search.SearchIssuesHandler
	ExploreIssue     mapview.ExploreIssueHandler
	GetPersonProfile people.GetPersonProfileHandler
	WorkCorrelations people.WorkCorrelationsHandler
}

// NewHandler creates a Streamable HTTP handler for the MCP server,
// suitable for mounting on an existing HTTP mux at a path like "/mcp".
func NewHandler(cfg ServerConfig) http.Handler {
	s := server.NewMCPServer(
		"sortit",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	h := &handlers{
		createIssue:      cfg.CreateIssue,
		refineIssues:     issuecmd.RefineIssuesHandler{RefineIssue: cfg.RefineIssue},
		progressIssues:   issuecmd.ProgressIssuesHandler{ProgressIssue: cfg.ProgressIssue},
		closeIssues:      issuecmd.CloseIssuesHandler{CloseIssue: cfg.CloseIssue},
		assignIssues:     issuecmd.AssignIssuesHandler{AssignIssue: cfg.AssignIssue},
		splitIssue:       cfg.SplitIssue,
		combineIssues:    cfg.CombineIssues,
		linkIssues:       cfg.LinkIssues,
		getIssue:         cfg.GetIssue,
		listTags:         cfg.ListTags,
		searchIssues:     cfg.SearchIssues,
		exploreIssue:     cfg.ExploreIssue,
		getPersonProfile: cfg.GetPersonProfile,
		workCorrelations: cfg.WorkCorrelations,
	}

	s.AddTool(
		mcp.NewTool("create_issue",
			mcp.WithDescription("Create a new issue in Sortit. Submit raw text (bug report, feature idea, stack trace, customer quote, etc.) and Sortit will automatically tag and categorize it using AI."),
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
			mcp.WithDescription("Search stored Sortit issues from arbitrary text. Returns the most related issues using a blend of semantic similarity, tag-factor relevance, and text matching."),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("The search text, page description, or other freeform query to match against stored issues."),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of related issues to return. Defaults to 8."),
			),
			mcp.WithNumber("offset",
				mcp.Description("Number of results to skip before returning, for pagination. Defaults to 0."),
			),
			mcp.WithString("status",
				mcp.Description("Optional issue status filter: open, closed, or all. Defaults to open."),
			),
			mcp.WithString("assigned_to",
				mcp.Description("Filter results to issues assigned to this person."),
			),
			mcp.WithString("tags",
				mcp.Description("Comma-separated tag names to filter results. Only issues matching at least one tag are returned."),
			),
			mcp.WithString("sort_by",
				mcp.Description("Sort order for results: relevance (default) or created_at (newest first)."),
			),
		),
		h.handleSearchIssues,
	)

	s.AddTool(
		mcp.NewTool("get_issue",
			mcp.WithDescription("Get a Sortit issue by ID."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("The issue ID, for example issue-000003."),
			),
		),
		h.handleGetIssue,
	)

	s.AddTool(
		mcp.NewTool("list_tags",
			mcp.WithDescription("List stored Sortit tags with descriptions, creation timestamps, and embeddings when available."),
		),
		h.handleListTags,
	)

	s.AddTool(
		mcp.NewTool("refine_issues",
			mcp.WithDescription("Refine one or more existing Sortit issues by appending shared discussion context or feedback. This updates each issue's canonical description, tags, and semantic similarity."),
			mcp.WithArray("ids",
				mcp.Required(),
				mcp.Description("The issue IDs to refine."),
				mcp.WithStringItems(),
			),
			mcp.WithString("raw",
				mcp.Required(),
				mcp.Description("The shared discussion post to append as additional context, refinement, or feedback."),
			),
			mcp.WithString("created_by",
				mcp.Description("Who authored the refinement. Defaults to 'Claude'."),
			),
		),
		h.handleRefineIssues,
	)

	s.AddTool(
		mcp.NewTool("progress_issues",
			mcp.WithDescription("Post the same progress update on one or more existing Sortit issues. Progress updates report work done toward resolving an issue without changing the canonical summary, tags, or semantic similarity."),
			mcp.WithArray("ids",
				mcp.Required(),
				mcp.Description("The issue IDs to post progress on."),
				mcp.WithStringItems(),
			),
			mcp.WithString("raw",
				mcp.Required(),
				mcp.Description("The shared progress update text describing work done toward each issue."),
			),
			mcp.WithString("created_by",
				mcp.Description("Who authored the progress update. Defaults to 'Claude'."),
			),
		),
		h.handleProgressIssues,
	)

	s.AddTool(
		mcp.NewTool("close_issues",
			mcp.WithDescription("Close one or more Sortit issues."),
			mcp.WithArray("ids",
				mcp.Required(),
				mcp.Description("The issue IDs to close."),
				mcp.WithStringItems(),
			),
			mcp.WithString("closed_by",
				mcp.Description("Who closed the issues. Defaults to 'Claude'."),
			),
		),
		h.handleCloseIssues,
	)

	s.AddTool(
		mcp.NewTool("assign_issues",
			mcp.WithDescription("Assign one or more Sortit issues to a person, or unassign by passing an empty string."),
			mcp.WithArray("ids",
				mcp.Required(),
				mcp.Description("The issue IDs to assign."),
				mcp.WithStringItems(),
			),
			mcp.WithString("assigned_to",
				mcp.Required(),
				mcp.Description("The person to assign every issue to. Pass an empty string to unassign."),
			),
		),
		h.handleAssignIssues,
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
			mcp.WithDescription("Explore a stored Sortit issue by ID. Returns related open issues using semantic similarity and factor relevance, plus structured opportunities to solve multiple issues together."),
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

	return server.NewStreamableHTTPServer(s)
}

type handlers struct {
	createIssue      issuecmd.CreateIssueHandler
	refineIssues     issuecmd.RefineIssuesHandler
	progressIssues   issuecmd.ProgressIssuesHandler
	closeIssues      issuecmd.CloseIssuesHandler
	assignIssues     issuecmd.AssignIssuesHandler
	splitIssue       issuecmd.SplitIssueHandler
	combineIssues    issuecmd.CombineIssuesHandler
	linkIssues       issuecmd.LinkIssuesHandler
	getIssue         issueviews.GetIssueHandler
	listTags         issueviews.ListTagsHandler
	searchIssues     search.SearchIssuesHandler
	exploreIssue     mapview.ExploreIssueHandler
	getPersonProfile people.GetPersonProfileHandler
	workCorrelations people.WorkCorrelationsHandler
}

func (h *handlers) handleCreateIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw, err := req.RequireString("raw")
	if err != nil {
		return mcp.NewToolResultError("raw is required"), nil //nolint:nilerr
	}

	issue, err := h.createIssue.Handle(ctx, issuecmd.CreateIssue{
		Raw:       strings.TrimSpace(raw),
		CreatedBy: actorForContext(ctx, req.GetString("created_by", "Claude")),
	})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(issue)
}

func (h *handlers) handleGetIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, result := requireTrimmedString(req, "id")
	if result != nil {
		return result, nil
	}

	issue, err := h.getIssue.Handle(ctx, issueviews.GetIssue{ID: id})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(issue)
}

func (h *handlers) handleListTags(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tags, err := h.listTags.Handle(ctx)
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(tags)
}

func (h *handlers) handleSearchIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil //nolint:nilerr
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	limit := req.GetInt("limit", 8)
	if limit <= 0 {
		return mcp.NewToolResultError("limit must be greater than 0"), nil
	}

	offset := req.GetInt("offset", 0)
	if offset < 0 {
		return mcp.NewToolResultError("offset must be >= 0"), nil
	}

	status, ok := parseStatusFilter(req.GetString("status", "open"), true)
	if !ok {
		return mcp.NewToolResultError("status must be one of: open, closed, all"), nil
	}

	assignedTo := strings.TrimSpace(req.GetString("assigned_to", ""))
	tagsRaw := strings.TrimSpace(req.GetString("tags", ""))
	var tags []string
	if tagsRaw != "" {
		for tag := range strings.SplitSeq(tagsRaw, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	sortBy := strings.TrimSpace(req.GetString("sort_by", "relevance"))
	if sortBy != "" && sortBy != "relevance" && sortBy != "created_at" {
		return mcp.NewToolResultError("sort_by must be one of: relevance, created_at"), nil
	}

	response, err := h.searchIssues.Handle(ctx, search.SearchIssues{
		Query:      query,
		Limit:      limit,
		Offset:     offset,
		Status:     status,
		AssignedTo: assignedTo,
		Tags:       tags,
		SortBy:     sortBy,
	})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(response)
}

func (h *handlers) handleRefineIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ids, result := requireIssueIDs(req)
	if result != nil {
		return result, nil
	}
	raw, result := requireTrimmedString(req, "raw")
	if result != nil {
		return result, nil
	}

	issue, err := h.refineIssues.Handle(ctx, issuecmd.RefineIssues{
		IDs:       ids,
		Raw:       raw,
		CreatedBy: actorForContext(ctx, req.GetString("created_by", "Claude")),
	})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(issue)
}

func (h *handlers) handleProgressIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ids, result := requireIssueIDs(req)
	if result != nil {
		return result, nil
	}
	raw, result := requireTrimmedString(req, "raw")
	if result != nil {
		return result, nil
	}

	issue, err := h.progressIssues.Handle(ctx, issuecmd.ProgressIssues{
		IDs:       ids,
		Raw:       raw,
		CreatedBy: actorForContext(ctx, req.GetString("created_by", "Claude")),
	})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(issue)
}

func (h *handlers) handleCloseIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ids, result := requireIssueIDs(req)
	if result != nil {
		return result, nil
	}

	issue, err := h.closeIssues.Handle(ctx, issuecmd.CloseIssues{
		IDs:      ids,
		ClosedBy: actorForContext(ctx, req.GetString("closed_by", "Claude")),
	})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(issue)
}

func (h *handlers) handleExploreIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, result := requireTrimmedString(req, "id")
	if result != nil {
		return result, nil
	}

	limit, result := requirePositiveInt(req, "limit", 8)
	if result != nil {
		return result, nil
	}

	response, err := h.exploreIssue.Handle(ctx, mapview.ExploreIssue{
		ID:    id,
		Limit: limit,
	})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(response)
}

func (h *handlers) handleAssignIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ids, result := requireIssueIDs(req)
	if result != nil {
		return result, nil
	}
	assignedTo, err := req.RequireString("assigned_to")
	if err != nil {
		return mcp.NewToolResultError("assigned_to is required"), nil //nolint:nilerr
	}

	issue, err := h.assignIssues.Handle(ctx, issuecmd.AssignIssues{
		IDs:        ids,
		AssignedTo: strings.TrimSpace(assignedTo),
	})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(issue)
}

func (h *handlers) handleSplitIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil //nolint:nilerr
	}
	children, err := req.RequireStringSlice("children_raw")
	if err != nil {
		return mcp.NewToolResultError("children_raw is required"), nil //nolint:nilerr
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	normalizedChildren := make([]issuecmd.SplitIssueChild, 0, len(children))
	for _, child := range children {
		child = strings.TrimSpace(child)
		if child == "" {
			continue
		}
		normalizedChildren = append(normalizedChildren, issuecmd.SplitIssueChild{Raw: child})
	}
	if len(normalizedChildren) == 0 {
		return mcp.NewToolResultError("children_raw must include at least one non-empty child"), nil
	}

	resultPayload, err := h.splitIssue.Handle(ctx, issuecmd.SplitIssue{
		SourceID:    id,
		Children:    normalizedChildren,
		CloseSource: req.GetBool("close_source", false),
		Note:        strings.TrimSpace(req.GetString("note", "")),
		CreatedBy:   actorForContext(ctx, req.GetString("created_by", "Claude")),
	})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(resultPayload)
}

func (h *handlers) handleCombineIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ids, err := req.RequireStringSlice("ids")
	if err != nil {
		return mcp.NewToolResultError("ids is required"), nil //nolint:nilerr
	}
	ids = issues.SanitizeIssueIDs(ids)
	if len(ids) < 2 {
		return mcp.NewToolResultError("at least two issue ids are required"), nil
	}

	resultPayload, err := h.combineIssues.Handle(ctx, issuecmd.CombineIssues{
		SourceIDs: ids,
		Note:      strings.TrimSpace(req.GetString("note", "")),
		CreatedBy: actorForContext(ctx, req.GetString("created_by", "Claude")),
	})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(resultPayload)
}

func (h *handlers) handleLinkIssues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceID, err := req.RequireString("source_id")
	if err != nil {
		return mcp.NewToolResultError("source_id is required"), nil //nolint:nilerr
	}
	targetID, err := req.RequireString("target_id")
	if err != nil {
		return mcp.NewToolResultError("target_id is required"), nil //nolint:nilerr
	}
	linkTypeRaw, err := req.RequireString("type")
	if err != nil {
		return mcp.NewToolResultError("type is required"), nil //nolint:nilerr
	}

	sourceID = strings.TrimSpace(sourceID)
	targetID = strings.TrimSpace(targetID)
	if sourceID == "" || targetID == "" {
		return mcp.NewToolResultError("source_id and target_id are required"), nil
	}

	linkType, ok := parseLinkType(linkTypeRaw)
	if !ok {
		return mcp.NewToolResultError("invalid link type"), nil
	}

	resultPayload, err := h.linkIssues.Handle(ctx, issuecmd.LinkIssues{
		SourceID:  sourceID,
		TargetID:  targetID,
		Type:      linkType,
		Note:      strings.TrimSpace(req.GetString("note", "")),
		CreatedBy: actorForContext(ctx, req.GetString("created_by", "Claude")),
	})
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(resultPayload)
}

func (h *handlers) handleGetPersonProfile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	person, result := requireTrimmedString(req, "person")
	if result != nil {
		return result, nil
	}

	status, ok := parseStatusFilter(req.GetString("status", "all"), false)
	if !ok {
		return mcp.NewToolResultError("status must be one of: open, closed, all"), nil
	}

	response, err := h.getPersonProfile.Handle(ctx, person, status)
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(response)
}

func (h *handlers) handleWorkCorrelations(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	status, ok := parseStatusFilter(req.GetString("status", "all"), false)
	if !ok {
		return mcp.NewToolResultError("status must be one of: open, closed, all"), nil
	}

	response, err := h.workCorrelations.Handle(ctx, status)
	if err != nil {
		return toolResultError(err), nil
	}

	return jsonResult(response)
}

func actorForContext(ctx context.Context, fallback string) string {
	if principal, ok := auth.PrincipalFromContext(ctx); ok {
		return principal.ActorName()
	}

	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "Claude"
	}
	return fallback
}

func requireTrimmedString(req mcp.CallToolRequest, field string) (string, *mcp.CallToolResult) {
	value, err := req.RequireString(field)
	if err != nil {
		return "", mcp.NewToolResultError(field + " is required")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", mcp.NewToolResultError(field + " is required")
	}
	return value, nil
}

func requirePositiveInt(req mcp.CallToolRequest, field string, fallback int) (int, *mcp.CallToolResult) {
	value := req.GetInt(field, fallback)
	if value <= 0 {
		return 0, mcp.NewToolResultError(field + " must be greater than 0")
	}
	return value, nil
}

func requireIssueIDs(req mcp.CallToolRequest) ([]string, *mcp.CallToolResult) {
	ids, err := req.RequireStringSlice("ids")
	if err != nil {
		return nil, mcp.NewToolResultError("ids is required")
	}
	ids = issues.SanitizeIssueIDs(ids)
	if len(ids) == 0 {
		return nil, mcp.NewToolResultError("at least one issue id is required")
	}
	return ids, nil
}

func parseStatusFilter(raw string, defaultOpen bool) (issueviews.IssueStatusFilter, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		if defaultOpen {
			return issueviews.IssueStatusFilterOpen, true
		}
		return issueviews.IssueStatusFilterAll, true
	case "open":
		return issueviews.IssueStatusFilterOpen, true
	case "closed":
		return issueviews.IssueStatusFilterClosed, true
	case "all":
		return issueviews.IssueStatusFilterAll, true
	default:
		return "", false
	}
}

func parseLinkType(raw string) (issues.IssueLinkType, bool) {
	linkType := issues.IssueLinkType(strings.TrimSpace(raw))
	switch linkType {
	case issues.IssueLinkTypeParentOf,
		issues.IssueLinkTypeChildOf,
		issues.IssueLinkTypeMergedInto,
		issues.IssueLinkTypeDerivedFrom,
		issues.IssueLinkTypeRelatedTo,
		issues.IssueLinkTypeDuplicateOf:
		return linkType, true
	default:
		return "", false
	}
}

func jsonResult[T any](payload T) (*mcp.CallToolResult, error) {
	result, err := mcp.NewToolResultJSON(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return result, nil
}

func toolResultError(err error) *mcp.CallToolResult {
	return mcp.NewToolResultError(toolErrorMessage(err))
}

func toolErrorMessage(err error) string {
	switch {
	case errors.Is(err, issues.ErrNotFound):
		return "issue not found"
	case errors.Is(err, issues.ErrIssueClosed):
		return "issue is closed"
	default:
		return err.Error()
	}
}
