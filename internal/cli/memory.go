package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"sortit/internal/domain"
)

type createMemoryRequest struct {
	Title          string   `json:"title,omitempty"`
	Body           string   `json:"body"`
	Kind           string   `json:"kind,omitempty"`
	AnchorTags     []string `json:"anchorTags,omitempty"`
	AnchorRegion   string   `json:"anchorRegion,omitempty"`
	CreatedBy      string   `json:"createdBy,omitempty"`
	SourceIssueIDs []string `json:"sourceIssueIds,omitempty"`
}

type memoriesListResponse struct {
	Memories []domain.Memory `json:"memories"`
}

// recalledMemory mirrors the API's memories.RecalledMemory: a memory plus its
// similarity to the recall query.
type recalledMemory struct {
	domain.Memory
	Similarity float64 `json:"similarity"`
}

type memoriesSearchResponse struct {
	Memories []recalledMemory `json:"memories"`
}

type supersedeMemoryRequest struct {
	SupersededBy string `json:"supersededBy,omitempty"`
}

func newMemoryCmd(opts *rootOptions) *cobra.Command {
	memoryCmd := &cobra.Command{
		Use:   "memory",
		Short: "Permanent knowledge artifacts (memories)",
	}
	memoryCmd.AddCommand(newMemoryCreateCmd(opts))
	memoryCmd.AddCommand(newMemoryGetCmd(opts))
	memoryCmd.AddCommand(newMemoryListCmd(opts))
	memoryCmd.AddCommand(newMemorySearchCmd(opts))
	memoryCmd.AddCommand(newMemorySupersedeCmd(opts))
	memoryCmd.AddCommand(newMemoryArchiveCmd(opts))
	memoryCmd.AddCommand(newMemoryProposalsCmd(opts))
	return memoryCmd
}

type memoryProposalsListResponse struct {
	Proposals []domain.MemoryProposal `json:"proposals"`
}

func newMemoryProposalsCmd(opts *rootOptions) *cobra.Command {
	proposalsCmd := &cobra.Command{
		Use:   "proposals",
		Short: "Synthesized memory proposals awaiting review",
	}
	proposalsCmd.AddCommand(newMemoryProposalsListCmd(opts))
	proposalsCmd.AddCommand(newMemoryProposalsSynthesizeCmd(opts))
	proposalsCmd.AddCommand(newMemoryProposalsAcceptCmd(opts))
	proposalsCmd.AddCommand(newMemoryProposalsRejectCmd(opts))
	return proposalsCmd
}

func newMemoryProposalsListCmd(opts *rootOptions) *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memory proposals",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			params := newQueryParams().add("status", status)
			var response memoryProposalsListResponse
			if err := client.Get(cmd.Context(), "/memories/proposals"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}
	cmd.Flags().StringVar(&status, "status", "pending", "Status filter: pending, accepted, rejected, all")
	return cmd
}

func newMemoryProposalsSynthesizeCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "synthesize",
		Short: "Scan the corpus and draft new memory proposals",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			var response memoryProposalsListResponse
			if err := client.Post(cmd.Context(), "/memories/proposals/synthesize", nil, &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}
}

func newMemoryProposalsAcceptCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "accept <id>",
		Short: "Accept a proposal, creating a permanent memory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var created domain.Memory
			if err := client.Post(cmd.Context(), "/memories/proposals/"+args[0]+"/accept", nil, &created); err != nil {
				return err
			}
			return printJSON(cmd, created)
		},
	}
}

func newMemoryProposalsRejectCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "reject <id>",
		Short: "Reject a proposal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var rejected domain.MemoryProposal
			if err := client.Post(cmd.Context(), "/memories/proposals/"+args[0]+"/reject", nil, &rejected); err != nil {
				return err
			}
			return printJSON(cmd, rejected)
		},
	}
}

func newMemoryCreateCmd(opts *rootOptions) *cobra.Command {
	var (
		title          string
		kind           string
		anchorTags     []string
		anchorRegion   string
		createdBy      string
		sourceIssueIDs []string
	)

	cmd := &cobra.Command{
		Use:   "create <body>",
		Short: "Create a memory (permanent, tag/region-anchored knowledge)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var created domain.Memory
			if err := client.Post(cmd.Context(), "/memories", createMemoryRequest{
				Title:          strings.TrimSpace(title),
				Body:           strings.TrimSpace(args[0]),
				Kind:           strings.TrimSpace(kind),
				AnchorTags:     normalizeCSV(anchorTags),
				AnchorRegion:   strings.TrimSpace(anchorRegion),
				CreatedBy:      strings.TrimSpace(createdBy),
				SourceIssueIDs: normalizeCSV(sourceIssueIDs),
			}, &created); err != nil {
				return err
			}
			return printJSON(cmd, created)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Short title shown as the memory's landmark label")
	cmd.Flags().StringVar(&kind, "kind", "", "Memory kind: decision, lesson, constraint, pattern, reference")
	cmd.Flags().StringSliceVar(&anchorTags, "anchor-tag", nil, "High-value tags this memory centers on")
	cmd.Flags().StringVar(&anchorRegion, "anchor-region", "", "Region/cluster id this memory anchors to")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "Override actor name when no authenticated principal is present")
	cmd.Flags().StringSliceVar(&sourceIssueIDs, "source-issue", nil, "Issue ids that taught us this memory")
	return cmd
}

func newMemoryGetCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Fetch a memory by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var memory domain.Memory
			if err := client.Get(cmd.Context(), "/memories/"+args[0], &memory); err != nil {
				return err
			}
			return printJSON(cmd, memory)
		},
	}
}

func newMemoryListCmd(opts *rootOptions) *cobra.Command {
	var (
		status string
		limit  int
		offset int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			params := newQueryParams().
				add("status", status).
				addInt("limit", limit).
				addInt("offset", offset)

			var response memoriesListResponse
			if err := client.Get(cmd.Context(), "/memories"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Status filter: active, superseded, archived, all")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of results")
	cmd.Flags().IntVar(&offset, "offset", 0, "Number of results to skip")
	return cmd
}

func newMemorySearchCmd(opts *rootOptions) *cobra.Command {
	var (
		limit         int
		minSimilarity float64
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Recall memories most relevant to a query",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			params := newQueryParams().
				add("q", strings.TrimSpace(args[0])).
				addInt("limit", limit).
				addFloat("min_similarity", minSimilarity)

			var response memoriesSearchResponse
			if err := client.Get(cmd.Context(), "/memories/search"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of results")
	cmd.Flags().Float64Var(&minSimilarity, "min-similarity", 0, "Drop results below this cosine similarity (0 to 1)")
	return cmd
}

func newMemorySupersedeCmd(opts *rootOptions) *cobra.Command {
	var supersededBy string

	cmd := &cobra.Command{
		Use:   "supersede <id>",
		Short: "Mark a memory as superseded by newer knowledge",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var updated domain.Memory
			if err := client.Post(cmd.Context(), "/memories/"+args[0]+"/supersede", supersedeMemoryRequest{
				SupersededBy: strings.TrimSpace(supersededBy),
			}, &updated); err != nil {
				return err
			}
			return printJSON(cmd, updated)
		},
	}

	cmd.Flags().StringVar(&supersededBy, "by", "", "Memory id that replaces this one")
	return cmd
}

func newMemoryArchiveCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "archive <id>",
		Short: "Archive a memory (retire without a replacement)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var updated domain.Memory
			if err := client.Post(cmd.Context(), "/memories/"+args[0]+"/archive", nil, &updated); err != nil {
				return err
			}
			return printJSON(cmd, updated)
		},
	}
}
