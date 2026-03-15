package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"splat/internal/cli/integrations"
	"splat/internal/commands"
	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/queries"
)

type rootOptions struct {
	apiURL     string
	token      string
	configPath string
}

type tagRecord struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Embedding   []float64 `json:"embedding,omitempty"`
}

type tagsResponse struct {
	Tags []tagRecord `json:"tags"`
}

type issuesListResponse struct {
	Issues []issues.Issue `json:"issues"`
}

type createIssueRequest struct {
	Raw       string   `json:"raw"`
	Tags      []string `json:"tags,omitempty"`
	CreatedBy string   `json:"createdBy,omitempty"`
}

type batchMutationRequest struct {
	IDs        []string `json:"ids"`
	Raw        string   `json:"raw,omitempty"`
	CreatedBy  string   `json:"createdBy,omitempty"`
	ClosedBy   string   `json:"closedBy,omitempty"`
	AssignedTo string   `json:"assignedTo,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	ReasonNote string   `json:"reasonNote,omitempty"`
}

type splitIssueChildRequest struct {
	Raw  string   `json:"raw"`
	Tags []string `json:"tags,omitempty"`
}

type splitIssueRequest struct {
	Children    []splitIssueChildRequest `json:"children"`
	CreatedBy   string                   `json:"createdBy,omitempty"`
	Note        string                   `json:"note,omitempty"`
	CloseSource bool                     `json:"closeSource"`
}

type combineIssuesRequest struct {
	IDs       []string `json:"ids"`
	CreatedBy string   `json:"createdBy,omitempty"`
	Note      string   `json:"note,omitempty"`
}

type linkIssuesRequest struct {
	SourceID  string `json:"sourceId"`
	TargetID  string `json:"targetId"`
	Type      string `json:"type"`
	CreatedBy string `json:"createdBy,omitempty"`
	Note      string `json:"note,omitempty"`
}

func NewRootCmd(version, commit, date string) *cobra.Command {
	opts := rootOptions{
		apiURL:     strings.TrimSpace(os.Getenv("SPLAT_API_URL")),
		token:      strings.TrimSpace(os.Getenv("SPLAT_API_TOKEN")),
		configPath: defaultConfigPath(),
	}

	rootCmd := &cobra.Command{
		Use:          "splat",
		Short:        "CLI for the Splat dedicated API",
		SilenceUsage: true,
		Long: `Splat CLI talks to the dedicated HTTP API.

Version: ` + version + `
Commit:  ` + commit + `
Date:    ` + date,
	}

	pf := rootCmd.PersistentFlags()
	pf.StringVar(&opts.apiURL, "api-url", opts.apiURL, "Base URL for the dedicated API. Defaults to saved config or http://localhost:8081/api/v1")
	pf.StringVar(&opts.token, "token", opts.token, "Bearer token for API access")
	pf.StringVar(&opts.configPath, "config", opts.configPath, "Path to the CLI config file")

	rootCmd.AddCommand(newAuthCmd(&opts))
	rootCmd.AddCommand(integrations.NewAgentsCmd(version))
	rootCmd.AddCommand(newMineCmd(&opts))
	rootCmd.AddCommand(newIssueCmd(&opts))
	rootCmd.AddCommand(newTagsCmd(&opts))
	rootCmd.AddCommand(newPeopleCmd(&opts))

	return rootCmd
}

func newIssueCmd(opts *rootOptions) *cobra.Command {
	issueCmd := &cobra.Command{
		Use:   "issues",
		Short: "Issue operations",
	}
	issueCmd.AddCommand(newIssueCreateCmd(opts))
	issueCmd.AddCommand(newIssueGetCmd(opts))
	issueCmd.AddCommand(newIssueSearchCmd(opts))
	issueCmd.AddCommand(newIssueRefineCmd(opts))
	issueCmd.AddCommand(newIssueProgressCmd(opts))
	issueCmd.AddCommand(newIssueCloseCmd(opts))
	issueCmd.AddCommand(newIssueAssignCmd(opts))
	issueCmd.AddCommand(newIssueSplitCmd(opts))
	issueCmd.AddCommand(newIssueCombineCmd(opts))
	issueCmd.AddCommand(newIssueLinkCmd(opts))
	issueCmd.AddCommand(newIssueExploreCmd(opts))
	return issueCmd
}

func newMineCmd(opts *rootOptions) *cobra.Command {
	var status string
	var limit int
	var offset int
	var tags []string

	cmd := &cobra.Command{
		Use:   "mine",
		Short: "List issues assigned to the current authenticated user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()

			var session authSessionResponse
			if err := client.Get(cmd.Context(), "/auth/session", &session); err != nil {
				return err
			}

			assignedTo := sessionActorName(session.User)
			if assignedTo == "" {
				return fmt.Errorf("authenticated user has no display name or login")
			}

			params := newQueryParams().
				add("status", status).
				addInt("limit", limit).
				addInt("offset", offset).
				add("assignedTo", assignedTo).
				addCSV("tags", tags)

			var response issuesListResponse
			if err := client.Get(cmd.Context(), "/issues"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}

	cmd.Flags().StringVar(&status, "status", "open", "Issue status filter: open, closed, all")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of results")
	cmd.Flags().IntVar(&offset, "offset", 0, "Number of results to skip")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "Restrict results to one or more tags")
	return cmd
}

func newIssueCreateCmd(opts *rootOptions) *cobra.Command {
	var createdBy string
	var tags []string

	cmd := &cobra.Command{
		Use:   "create <raw>",
		Short: "Create an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var created issues.Issue
			if err := client.Post(cmd.Context(), "/issues", createIssueRequest{
				Raw:       strings.TrimSpace(args[0]),
				CreatedBy: strings.TrimSpace(createdBy),
				Tags:      normalizeCSV(tags),
			}, &created); err != nil {
				return err
			}
			return printJSON(cmd, created)
		},
	}

	cmd.Flags().StringVar(&createdBy, "created-by", "", "Override actor name when no authenticated principal is present")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "Attach one or more tags")
	return cmd
}

func newIssueGetCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Fetch an issue by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var issue issues.Issue
			if err := client.Get(cmd.Context(), "/issues/"+args[0], &issue); err != nil {
				return err
			}
			return printJSON(cmd, issue)
		},
	}
}

func newIssueSearchCmd(opts *rootOptions) *cobra.Command {
	var status string
	var limit int
	var offset int
	var assignedTo string
	var tags []string
	var sortBy string

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search issues",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			params := newQueryParams().
				add("q", args[0]).
				add("status", status).
				addInt("limit", limit).
				addInt("offset", offset).
				add("assignedTo", assignedTo).
				add("sortBy", sortBy).
				addCSV("tags", tags)

			var response issuemap.SearchResponse
			if err := client.Get(cmd.Context(), "/issues/search"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}

	cmd.Flags().StringVar(&status, "status", "open", "Issue status filter: open, closed, all")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of results")
	cmd.Flags().IntVar(&offset, "offset", 0, "Number of results to skip")
	cmd.Flags().StringVar(&assignedTo, "assigned-to", "", "Restrict results to a person")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "Restrict results to one or more tags")
	cmd.Flags().StringVar(&sortBy, "sort-by", "", "Sort results by relevance or created_at")
	return cmd
}

func newIssueRefineCmd(opts *rootOptions) *cobra.Command {
	var raw string
	var createdBy string

	cmd := &cobra.Command{
		Use:   "refine <id> [id...]",
		Short: "Append a refinement to one or more issues",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var result commands.IssueMutationResult
			if err := client.Post(cmd.Context(), "/issues/refine", batchMutationRequest{
				IDs:       args,
				Raw:       raw,
				CreatedBy: createdBy,
			}, &result); err != nil {
				return err
			}
			return printJSON(cmd, result)
		},
	}

	cmd.Flags().StringVar(&raw, "raw", "", "Refinement text")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "Override actor name when no authenticated principal is present")
	_ = cmd.MarkFlagRequired("raw")
	return cmd
}

func newIssueProgressCmd(opts *rootOptions) *cobra.Command {
	var raw string
	var createdBy string

	cmd := &cobra.Command{
		Use:   "progress <id> [id...]",
		Short: "Append a progress update to one or more issues",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var result commands.IssueMutationResult
			if err := client.Post(cmd.Context(), "/issues/progress", batchMutationRequest{
				IDs:       args,
				Raw:       raw,
				CreatedBy: createdBy,
			}, &result); err != nil {
				return err
			}
			return printJSON(cmd, result)
		},
	}

	cmd.Flags().StringVar(&raw, "raw", "", "Progress update text")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "Override actor name when no authenticated principal is present")
	_ = cmd.MarkFlagRequired("raw")
	return cmd
}

func newIssueCloseCmd(opts *rootOptions) *cobra.Command {
	var closedBy string
	var reason string
	var reasonNote string

	cmd := &cobra.Command{
		Use:   "close <id> [id...]",
		Short: "Close one or more issues",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var result commands.IssueMutationResult
			if err := client.Post(cmd.Context(), "/issues/close", batchMutationRequest{
				IDs:        args,
				ClosedBy:   closedBy,
				Reason:     reason,
				ReasonNote: reasonNote,
			}, &result); err != nil {
				return err
			}
			return printJSON(cmd, result)
		},
	}

	cmd.Flags().StringVar(&closedBy, "closed-by", "", "Override actor name when no authenticated principal is present")
	cmd.Flags().StringVar(&reason, "reason", "", "Close reason: fixed, stale, duplicate, wont_fix, by_design")
	cmd.Flags().StringVar(&reasonNote, "reason-note", "", "Optional note explaining the close reason")
	return cmd
}

func newIssueAssignCmd(opts *rootOptions) *cobra.Command {
	var assignedTo string
	var createdBy string

	cmd := &cobra.Command{
		Use:   "assign <id> [id...]",
		Short: "Assign or unassign one or more issues",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var result commands.IssueMutationResult
			if err := client.Post(cmd.Context(), "/issues/assign", batchMutationRequest{
				IDs:        args,
				AssignedTo: assignedTo,
				CreatedBy:  createdBy,
			}, &result); err != nil {
				return err
			}
			return printJSON(cmd, result)
		},
	}

	cmd.Flags().StringVar(&assignedTo, "assigned-to", "", "Person to assign. Pass an empty string to unassign")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "Override actor name when no authenticated principal is present")
	return cmd
}

func newIssueSplitCmd(opts *rootOptions) *cobra.Command {
	var children []string
	var childTags []string
	var createdBy string
	var note string
	var closeSource bool

	cmd := &cobra.Command{
		Use:   "split <id>",
		Short: "Split an issue into child issues",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			requestChildren := make([]splitIssueChildRequest, 0, len(children))
			tags := normalizeCSV(childTags)
			for _, child := range children {
				requestChildren = append(requestChildren, splitIssueChildRequest{
					Raw:  strings.TrimSpace(child),
					Tags: tags,
				})
			}

			var result issues.IssueOperationResult
			if err := client.Post(cmd.Context(), "/issues/"+args[0]+"/split", splitIssueRequest{
				Children:    requestChildren,
				CreatedBy:   createdBy,
				Note:        note,
				CloseSource: closeSource,
			}, &result); err != nil {
				return err
			}
			return printJSON(cmd, result)
		},
	}

	cmd.Flags().StringArrayVar(&children, "child", nil, "Child issue raw text. Repeat for multiple children")
	cmd.Flags().StringSliceVar(&childTags, "child-tag", nil, "Optional tags applied to every child")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "Override actor name when no authenticated principal is present")
	cmd.Flags().StringVar(&note, "note", "", "Optional note attached to the split operation")
	cmd.Flags().BoolVar(&closeSource, "close-source", false, "Close the source issue after splitting")
	_ = cmd.MarkFlagRequired("child")
	return cmd
}

func newIssueCombineCmd(opts *rootOptions) *cobra.Command {
	var createdBy string
	var note string

	cmd := &cobra.Command{
		Use:   "combine <id> <id> [id...]",
		Short: "Combine multiple issues into a canonical issue",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var result issues.IssueOperationResult
			if err := client.Post(cmd.Context(), "/issues/combine", combineIssuesRequest{
				IDs:       args,
				CreatedBy: createdBy,
				Note:      note,
			}, &result); err != nil {
				return err
			}
			return printJSON(cmd, result)
		},
	}

	cmd.Flags().StringVar(&createdBy, "created-by", "", "Override actor name when no authenticated principal is present")
	cmd.Flags().StringVar(&note, "note", "", "Optional note attached to the combine operation")
	return cmd
}

func newIssueLinkCmd(opts *rootOptions) *cobra.Command {
	var sourceID string
	var targetID string
	var linkType string
	var createdBy string
	var note string

	cmd := &cobra.Command{
		Use:   "link",
		Short: "Link two issues",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			var result issues.IssueOperationResult
			if err := client.Post(cmd.Context(), "/issues/link", linkIssuesRequest{
				SourceID:  sourceID,
				TargetID:  targetID,
				Type:      linkType,
				CreatedBy: createdBy,
				Note:      note,
			}, &result); err != nil {
				return err
			}
			return printJSON(cmd, result)
		},
	}

	cmd.Flags().StringVar(&sourceID, "source", "", "Source issue ID")
	cmd.Flags().StringVar(&targetID, "target", "", "Target issue ID")
	cmd.Flags().StringVar(&linkType, "type", "", "Link type: parent_of, child_of, merged_into, derived_from, related_to, duplicate_of")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "Override actor name when no authenticated principal is present")
	cmd.Flags().StringVar(&note, "note", "", "Optional note attached to the link")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("target")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

func newIssueExploreCmd(opts *rootOptions) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "explore <id>",
		Short: "Explore related issues for a given issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			params := newQueryParams().addInt("limit", limit)
			var response issuemap.ExploreResponse
			if err := client.Get(cmd.Context(), "/issues/"+args[0]+"/explore"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of related issues")
	return cmd
}

func newTagsCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tags",
		Short: "Tag operations",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List tags",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			var response tagsResponse
			if err := client.Get(cmd.Context(), "/tags", &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	})
	return cmd
}

func newPeopleCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "people",
		Short: "People analytics operations",
	}
	cmd.AddCommand(newPeopleProfileCmd(opts))
	cmd.AddCommand(newPeopleCorrelationsCmd(opts))
	return cmd
}

func newPeopleProfileCmd(opts *rootOptions) *cobra.Command {
	var status string

	cmd := &cobra.Command{
		Use:   "profile <person>",
		Short: "Get a person's tag profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			params := newQueryParams().add("status", status)
			var response queries.PersonTagProfile
			if err := client.Get(cmd.Context(), "/people/"+args[0]+"/profile"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}

	cmd.Flags().StringVar(&status, "status", "all", "Issue status filter: open, closed, all")
	return cmd
}

func newPeopleCorrelationsCmd(opts *rootOptions) *cobra.Command {
	var status string

	cmd := &cobra.Command{
		Use:   "correlations",
		Short: "Find overlapping work across people",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			params := newQueryParams().add("status", status)
			var response queries.WorkCorrelationsResult
			if err := client.Get(cmd.Context(), "/people/correlations"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}

	cmd.Flags().StringVar(&status, "status", "all", "Issue status filter: open, closed, all")
	return cmd
}

func (o *rootOptions) client() *apiClient {
	return newAPIClient(o.resolvedAPIURL(), o.resolvedToken())
}

func (o *rootOptions) resolvedAPIURL() string {
	if value := strings.TrimSpace(o.apiURL); value != "" {
		return value
	}
	cfg, err := loadCLIConfig(o.configPath)
	if err == nil && strings.TrimSpace(cfg.APIURL) != "" {
		return strings.TrimSpace(cfg.APIURL)
	}
	return "http://localhost:8081/api/v1"
}

func (o *rootOptions) resolvedToken() string {
	if value := strings.TrimSpace(o.token); value != "" {
		return value
	}
	cfg, err := loadCLIConfig(o.configPath)
	if err == nil {
		return strings.TrimSpace(cfg.Token)
	}
	return ""
}

func printJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func sessionActorName(user SessionUser) string {
	if value := strings.TrimSpace(user.DisplayName); value != "" {
		return value
	}
	return strings.TrimSpace(user.Login)
}

func normalizeCSV(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			out = append(out, part)
		}
	}
	return out
}

type queryParams map[string]string

func newQueryParams() queryParams {
	return make(queryParams)
}

func (q queryParams) add(key, value string) queryParams {
	value = strings.TrimSpace(value)
	if value != "" {
		q[key] = value
	}
	return q
}

func (q queryParams) addInt(key string, value int) queryParams {
	if value > 0 {
		q[key] = fmt.Sprintf("%d", value)
	}
	return q
}

func (q queryParams) addCSV(key string, values []string) queryParams {
	normalized := normalizeCSV(values)
	if len(normalized) > 0 {
		q[key] = strings.Join(normalized, ",")
	}
	return q
}

func (q queryParams) encode() string {
	if len(q) == 0 {
		return ""
	}

	keys := make([]string, 0, len(q))
	for key := range q {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	var builder strings.Builder
	builder.WriteByte('?')
	for i, key := range keys {
		if i > 0 {
			builder.WriteByte('&')
		}
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(url.QueryEscape(q[key]))
	}
	return builder.String()
}
