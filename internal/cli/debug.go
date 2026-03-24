package cli

import (
	"time"

	"github.com/spf13/cobra"

	"splat/internal/queries"
)

const debugEvalTagsClientTimeout = 2 * time.Minute

func newDebugCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debug",
		Short: "Diagnostic and debugging tools",
	}
	cmd.AddCommand(newDebugEvalTagsCmd(opts))
	cmd.AddCommand(newDebugFactorWeightsCmd(opts))
	cmd.AddCommand(newDebugIssueR2Cmd(opts))
	return cmd
}

func newDebugEvalTagsCmd(opts *rootOptions) *cobra.Command {
	var fixture string
	var mode string
	var verify bool
	var runs int

	cmd := &cobra.Command{
		Use:   "eval-tags",
		Short: "Run the built-in tag benchmark and report tagging diffs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.clientWithTimeout(debugEvalTagsClientTimeout)
			var response queries.DebugEvalTagsResult
			params := newQueryParams().add("fixture", fixture).add("mode", mode).addBool("verify", verify).addInt("runs", runs)
			if err := client.Get(cmd.Context(), "/debug/eval-tags"+params.encode(), &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}

	cmd.Flags().StringVar(&fixture, "fixture", "", "Benchmark fixture to run: corpus (default) or synthetic")
	cmd.Flags().StringVar(&mode, "mode", "retrieval-shortlist", "Candidate mode to evaluate: retrieval-shortlist (default) or full-catalog")
	cmd.Flags().BoolVar(&verify, "verify", true, "Enable deterministic post-LLM tag verification")
	cmd.Flags().IntVar(&runs, "runs", 1, "How many times to re-run each case for stability measurement")
	return cmd
}

func newDebugFactorWeightsCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "factor-weights",
		Short: "Show factor decomposition weights and low-R² issues",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			var response queries.DebugFactorWeightsResult
			if err := client.Get(cmd.Context(), "/debug/factor-weights", &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}
}

func newDebugIssueR2Cmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "issue-r2 <id>",
		Short: "Diagnose why an issue has low R² in the factor model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			var response queries.DebugIssueR2Result
			if err := client.Get(cmd.Context(), "/debug/issues/"+args[0]+"/r2", &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}
}
