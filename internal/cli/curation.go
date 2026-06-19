package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"sortit/internal/domain"
)

type curationProposalsListResponse struct {
	Proposals []domain.CurationProposal `json:"proposals"`
}

type createCurationProposalRequest struct {
	Kind       string                 `json:"kind"`
	Payload    domain.CurationPayload `json:"payload"`
	Rationale  string                 `json:"rationale,omitempty"`
	Confidence float64                `json:"confidence,omitempty"`
	SourceRefs []string               `json:"sourceRefs,omitempty"`
	CreatedBy  string                 `json:"createdBy,omitempty"`
}

func newCurationCmd(opts *rootOptions) *cobra.Command {
	curationCmd := &cobra.Command{
		Use:   "curation",
		Short: "Librarian review queue: curate issues and memories",
	}
	curationCmd.AddCommand(newCurationCandidatesCmd(opts))
	curationCmd.AddCommand(newCurationProposalsCmd(opts))
	return curationCmd
}

func newCurationCandidatesCmd(opts *rootOptions) *cobra.Command {
	candidatesCmd := &cobra.Command{
		Use:   "candidates",
		Short: "Read-only curation candidates for the librarian to judge",
	}
	candidatesCmd.AddCommand(newCurationCandidatesDuplicatesCmd(opts))
	candidatesCmd.AddCommand(newCurationCandidatesStaleCmd(opts))
	return candidatesCmd
}

func newCurationCandidatesDuplicatesCmd(opts *rootOptions) *cobra.Command {
	var (
		minSimilarity float64
		maxSeeds      int
		exploreLimit  int
		limit         int
	)
	cmd := &cobra.Command{
		Use:   "duplicates",
		Short: "Clusters of highly-similar open issues worth combining",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			params := newQueryParams().
				addFloat("minSimilarity", minSimilarity).
				addInt("maxSeeds", maxSeeds).
				addInt("exploreLimit", exploreLimit).
				addInt("limit", limit)
			var response map[string]any
			if err := client.Get(cmd.Context(), "/curation/candidates/duplicates"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}
	cmd.Flags().Float64Var(&minSimilarity, "min-similarity", 0, "Minimum combined similarity to link two issues (default 0.85)")
	cmd.Flags().IntVar(&maxSeeds, "max-seeds", 0, "Cap seed issues explored (0 = all open issues)")
	cmd.Flags().IntVar(&exploreLimit, "explore-limit", 0, "Per-seed related-issue limit (default 10)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum clusters returned")
	return cmd
}

func newCurationCandidatesStaleCmd(opts *rootOptions) *cobra.Command {
	var (
		minDaysInactive float64
		maxVelocity     float64
		limit           int
	)
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Long-inactive open issues worth closing as stale",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			params := newQueryParams().
				addFloat("minDaysInactive", minDaysInactive).
				addFloat("maxVelocity", maxVelocity).
				addInt("limit", limit)
			var response map[string]any
			if err := client.Get(cmd.Context(), "/curation/candidates/stale"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}
	cmd.Flags().Float64Var(&minDaysInactive, "min-days-inactive", 0, "Minimum days since last activity (default 90)")
	cmd.Flags().Float64Var(&maxVelocity, "max-velocity", 0, "Maximum velocity to still count as stale (default 0.05)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum results returned")
	return cmd
}

func newCurationProposalsCmd(opts *rootOptions) *cobra.Command {
	proposalsCmd := &cobra.Command{
		Use:   "proposals",
		Short: "Curation moves awaiting review (the librarian proposes, a human decides)",
	}
	proposalsCmd.AddCommand(newCurationProposalsListCmd(opts))
	proposalsCmd.AddCommand(newCurationProposalsGetCmd(opts))
	proposalsCmd.AddCommand(newCurationProposalsCreateCmd(opts))
	proposalsCmd.AddCommand(newCurationProposalsAcceptCmd(opts))
	proposalsCmd.AddCommand(newCurationProposalsRejectCmd(opts))
	return proposalsCmd
}

func newCurationProposalsListCmd(opts *rootOptions) *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List curation proposals",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			params := newQueryParams().add("status", status)
			var response curationProposalsListResponse
			if err := client.Get(cmd.Context(), "/curation/proposals"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}
	cmd.Flags().StringVar(&status, "status", "pending", "Status filter: pending, accepted, rejected, all")
	return cmd
}

func newCurationProposalsGetCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Fetch a curation proposal by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var proposal domain.CurationProposal
			if err := client.Get(cmd.Context(), "/curation/proposals/"+args[0], &proposal); err != nil {
				return err
			}
			return printJSON(cmd, proposal)
		},
	}
}

func newCurationProposalsCreateCmd(opts *rootOptions) *cobra.Command {
	var (
		kind          string
		issueIDs      []string
		canonicalID   string
		reason        string
		note          string
		memoryID      string
		replacementID string
		rationale     string
		confidence    float64
		sourceRefs    []string
		createdBy     string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Draft a curation move for review",
		Long: "Draft a curation move for review. Kinds: combine_issues (--issue x2+, " +
			"--canonical), close_stale (--issue, --reason, --note), reenrich_issue (--issue), " +
			"archive_memory (--memory), supersede_memory (--memory, --replacement).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			ids := normalizeCSV(issueIDs)
			payload := domain.CurationPayload{
				IssueIDs:      ids,
				CanonicalID:   strings.TrimSpace(canonicalID),
				Reason:        strings.TrimSpace(reason),
				Note:          strings.TrimSpace(note),
				MemoryID:      strings.TrimSpace(memoryID),
				ReplacementID: strings.TrimSpace(replacementID),
			}
			// Single-target kinds read Payload.IssueID; populate it from the
			// first --issue so the same flag serves combine and close/reenrich.
			if len(ids) > 0 {
				payload.IssueID = ids[0]
			}

			var created domain.CurationProposal
			if err := client.Post(cmd.Context(), "/curation/proposals", createCurationProposalRequest{
				Kind:       strings.TrimSpace(kind),
				Payload:    payload,
				Rationale:  strings.TrimSpace(rationale),
				Confidence: confidence,
				SourceRefs: normalizeCSV(sourceRefs),
				CreatedBy:  strings.TrimSpace(createdBy),
			}, &created); err != nil {
				return err
			}
			return printJSON(cmd, created)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Curation kind: combine_issues, close_stale, reenrich_issue, archive_memory, supersede_memory")
	cmd.Flags().StringSliceVar(&issueIDs, "issue", nil, "Issue id(s) the move targets (repeat or comma-separate for combine_issues)")
	cmd.Flags().StringVar(&canonicalID, "canonical", "", "Advisory canonical issue id for combine_issues")
	cmd.Flags().StringVar(&reason, "reason", "", "Close reason for close_stale (default stale)")
	cmd.Flags().StringVar(&note, "note", "", "Close note for close_stale")
	cmd.Flags().StringVar(&memoryID, "memory", "", "Memory id for archive_memory or supersede_memory")
	cmd.Flags().StringVar(&replacementID, "replacement", "", "Replacement memory id for supersede_memory")
	cmd.Flags().StringVar(&rationale, "rationale", "", "Why the librarian proposes this move")
	cmd.Flags().Float64Var(&confidence, "confidence", 0, "Confidence in the move [0,1]")
	cmd.Flags().StringSliceVar(&sourceRefs, "source-ref", nil, "Issue/memory ids cited as evidence")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "Override actor name when no authenticated principal is present")
	_ = cmd.MarkFlagRequired("kind")
	return cmd
}

func newCurationProposalsAcceptCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "accept <id>",
		Short: "Accept a proposal, applying the curation move",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var accepted domain.CurationProposal
			if err := client.Post(cmd.Context(), "/curation/proposals/"+args[0]+"/accept", nil, &accepted); err != nil {
				return err
			}
			return printJSON(cmd, accepted)
		},
	}
}

func newCurationProposalsRejectCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "reject <id>",
		Short: "Reject a proposal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var rejected domain.CurationProposal
			if err := client.Post(cmd.Context(), "/curation/proposals/"+args[0]+"/reject", nil, &rejected); err != nil {
				return err
			}
			return printJSON(cmd, rejected)
		},
	}
}
