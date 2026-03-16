package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"splat/internal/issues"
	"splat/internal/issues/appendonly"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "append-only migration command failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	baseURL := strings.TrimSpace(firstNonEmpty(
		os.Getenv("SPLAT_DATABASE_URL"),
		os.Getenv("SPLAT_TEST_DATABASE_URL"),
	))
	if baseURL == "" {
		return fmt.Errorf("set SPLAT_DATABASE_URL or SPLAT_TEST_DATABASE_URL")
	}

	store, err := issues.OpenPostgresStore(ctx, baseURL)
	if err != nil {
		return err
	}
	defer store.Close() //nolint:errcheck

	framework := appendonly.NewFrameworkStore(store.DB())
	lifecycle := appendonly.NewLifecycleStore(store.DB())
	enrichment := appendonly.NewEnrichmentStore(store.DB())
	content := appendonly.NewIssueContentStore(store.DB())
	tags := appendonly.NewTagStore(store.DB())

	switch args[0] {
	case "status":
		return runStatus(ctx, framework, args[1:])
	case "checkpoint":
		return runCheckpoint(ctx, framework, args[1:])
	case "parity":
		return runParity(ctx, framework, args[1:])
	case "issue-lifecycle-backfill":
		return runIssueLifecycleBackfill(ctx, lifecycle, store, args[1:])
	case "issue-lifecycle-parity":
		return runIssueLifecycleParity(ctx, framework, lifecycle, store, args[1:])
	case "issue-enrichment-backfill":
		return runIssueEnrichmentBackfill(ctx, enrichment, args[1:])
	case "issue-enrichment-parity":
		return runIssueEnrichmentParity(ctx, framework, enrichment, args[1:])
	case "issue-content-backfill":
		return runIssueContentBackfill(ctx, content, args[1:])
	case "issue-content-parity":
		return runIssueContentParity(ctx, framework, content, args[1:])
	case "tag-backfill":
		return runTagBackfill(ctx, tags, args[1:])
	case "tag-parity":
		return runTagParity(ctx, framework, tags, args[1:])
	default:
		return usageError()
	}
}

func runStatus(ctx context.Context, framework *appendonly.FrameworkStore, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		domain string
		limit  int
	)
	fs.StringVar(&domain, "domain", "", "Limit parity runs to a specific domain")
	fs.IntVar(&limit, "limit", 20, "Maximum number of parity runs to show")
	if err := fs.Parse(args); err != nil {
		return err
	}

	checkpoints, err := framework.ListCheckpoints(ctx)
	if err != nil {
		return err
	}
	runs, err := framework.ListParityRuns(ctx, domain, limit)
	if err != nil {
		return err
	}

	return printJSON(struct {
		Checkpoints []appendonly.Checkpoint `json:"checkpoints"`
		ParityRuns  []appendonly.ParityRun  `json:"parityRuns"`
	}{
		Checkpoints: checkpoints,
		ParityRuns:  runs,
	})
}

func runCheckpoint(ctx context.Context, framework *appendonly.FrameworkStore, args []string) error {
	fs := flag.NewFlagSet("checkpoint", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		name        string
		phase       string
		cursorJSON  string
		summaryJSON string
	)
	fs.StringVar(&name, "name", "", "Checkpoint name")
	fs.StringVar(&phase, "phase", "", "Current phase label")
	fs.StringVar(&cursorJSON, "cursor-json", "{}", "Checkpoint cursor JSON object")
	fs.StringVar(&summaryJSON, "summary-json", "{}", "Checkpoint summary JSON object")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("checkpoint --name is required")
	}

	if err := framework.UpsertCheckpoint(ctx, appendonly.Checkpoint{
		Name:        name,
		Phase:       phase,
		CursorJSON:  json.RawMessage(cursorJSON),
		SummaryJSON: json.RawMessage(summaryJSON),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		return err
	}

	return runStatus(ctx, framework, nil)
}

func runParity(ctx context.Context, framework *appendonly.FrameworkStore, args []string) error {
	fs := flag.NewFlagSet("parity", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		id          string
		domain      string
		status      string
		detailsJSON string
	)
	fs.StringVar(&id, "id", "", "Parity run id (default: generated)")
	fs.StringVar(&domain, "domain", "", "Parity domain, e.g. issue-lifecycle")
	fs.StringVar(&status, "status", "", "Parity status label, e.g. baseline or pass")
	fs.StringVar(&detailsJSON, "details-json", "{}", "Parity run details JSON object")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(domain) == "" {
		return errors.New("parity --domain is required")
	}
	if strings.TrimSpace(status) == "" {
		return errors.New("parity --status is required")
	}
	if strings.TrimSpace(id) == "" {
		id = "parity-" + randomSuffix()
	}

	if err := framework.RecordParityRun(ctx, appendonly.ParityRun{
		ID:          id,
		Domain:      domain,
		Status:      status,
		DetailsJSON: json.RawMessage(detailsJSON),
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		return err
	}

	return runStatus(ctx, framework, []string{"--domain", domain})
}

func runIssueLifecycleBackfill(
	ctx context.Context,
	lifecycle *appendonly.LifecycleStore,
	source interface {
		List(context.Context) ([]issues.Issue, error)
		GetIssueDetail(context.Context, string) (issues.Issue, error)
	},
	args []string,
) error {
	fs := flag.NewFlagSet("issue-lifecycle-backfill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		batchSize  int
		checkpoint string
	)
	fs.IntVar(&batchSize, "batch-size", 100, "Maximum number of issues to backfill in one batch")
	fs.StringVar(&checkpoint, "checkpoint", "issue-lifecycle-backfill", "Checkpoint name to update")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := lifecycle.BackfillBatch(ctx, source, checkpoint, batchSize)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func runIssueLifecycleParity(
	ctx context.Context,
	framework *appendonly.FrameworkStore,
	lifecycle *appendonly.LifecycleStore,
	source interface {
		List(context.Context) ([]issues.Issue, error)
		GetIssueDetail(context.Context, string) (issues.Issue, error)
	},
	args []string,
) error {
	fs := flag.NewFlagSet("issue-lifecycle-parity", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		detailLimit int
		recordID    string
	)
	fs.IntVar(&detailLimit, "detail-limit", 20, "Maximum number of mismatches to include in the output")
	fs.StringVar(&recordID, "record-id", "", "Parity run id (default: generated)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := lifecycle.CheckParity(ctx, source, detailLimit)
	if err != nil {
		return err
	}

	status := "pass"
	if result.MissingProjections > 0 || result.MismatchedIssues > 0 {
		status = "fail"
	}
	if strings.TrimSpace(recordID) == "" {
		recordID = "parity-" + randomSuffix()
	}

	if err := framework.RecordParityRun(ctx, appendonly.ParityRun{
		ID:          recordID,
		Domain:      "issue-lifecycle",
		Status:      status,
		DetailsJSON: mustJSON(result),
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		return err
	}

	return printJSON(struct {
		RunID  string                           `json:"runId"`
		Status string                           `json:"status"`
		Result appendonly.LifecycleParityResult `json:"result"`
	}{
		RunID:  recordID,
		Status: status,
		Result: result,
	})
}

func runIssueEnrichmentBackfill(
	ctx context.Context,
	enrichment *appendonly.EnrichmentStore,
	args []string,
) error {
	fs := flag.NewFlagSet("issue-enrichment-backfill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		batchSize  int
		checkpoint string
	)
	fs.IntVar(&batchSize, "batch-size", 100, "Maximum number of issues to backfill in one batch")
	fs.StringVar(&checkpoint, "checkpoint", "issue-enrichment-backfill", "Checkpoint name to update")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := enrichment.BackfillBatch(ctx, checkpoint, batchSize)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func runIssueEnrichmentParity(
	ctx context.Context,
	framework *appendonly.FrameworkStore,
	enrichment *appendonly.EnrichmentStore,
	args []string,
) error {
	fs := flag.NewFlagSet("issue-enrichment-parity", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		detailLimit int
		recordID    string
	)
	fs.IntVar(&detailLimit, "detail-limit", 20, "Maximum number of mismatches to include in the output")
	fs.StringVar(&recordID, "record-id", "", "Parity run id (default: generated)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := enrichment.CheckParity(ctx, detailLimit)
	if err != nil {
		return err
	}

	status := "pass"
	if result.MissingProjections > 0 || result.MismatchedIssues > 0 {
		status = "fail"
	}
	if strings.TrimSpace(recordID) == "" {
		recordID = "parity-" + randomSuffix()
	}

	if err := framework.RecordParityRun(ctx, appendonly.ParityRun{
		ID:          recordID,
		Domain:      "issue-enrichment",
		Status:      status,
		DetailsJSON: mustJSON(result),
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		return err
	}

	return printJSON(struct {
		RunID  string                            `json:"runId"`
		Status string                            `json:"status"`
		Result appendonly.EnrichmentParityResult `json:"result"`
	}{
		RunID:  recordID,
		Status: status,
		Result: result,
	})
}

func printJSON(value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func runTagBackfill(
	ctx context.Context,
	tags *appendonly.TagStore,
	args []string,
) error {
	fs := flag.NewFlagSet("tag-backfill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		batchSize  int
		checkpoint string
	)
	fs.IntVar(&batchSize, "batch-size", 100, "Maximum number of tags to backfill in one batch")
	fs.StringVar(&checkpoint, "checkpoint", "tag-backfill", "Checkpoint name to update")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := tags.BackfillBatch(ctx, checkpoint, batchSize)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func runIssueContentBackfill(
	ctx context.Context,
	content *appendonly.IssueContentStore,
	args []string,
) error {
	fs := flag.NewFlagSet("issue-content-backfill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		batchSize  int
		checkpoint string
	)
	fs.IntVar(&batchSize, "batch-size", 100, "Maximum number of issues to backfill in one batch")
	fs.StringVar(&checkpoint, "checkpoint", "issue-content-backfill", "Checkpoint name to update")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := content.BackfillBatch(ctx, checkpoint, batchSize)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func runIssueContentParity(
	ctx context.Context,
	framework *appendonly.FrameworkStore,
	content *appendonly.IssueContentStore,
	args []string,
) error {
	fs := flag.NewFlagSet("issue-content-parity", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		detailLimit int
		recordID    string
	)
	fs.IntVar(&detailLimit, "detail-limit", 20, "Maximum number of mismatches to include in the output")
	fs.StringVar(&recordID, "record-id", "", "Parity run id (default: generated)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := content.CheckParity(ctx, detailLimit)
	if err != nil {
		return err
	}

	status := "pass"
	if result.MissingProjections > 0 || result.MismatchedIssues > 0 {
		status = "fail"
	}
	if strings.TrimSpace(recordID) == "" {
		recordID = "parity-" + randomSuffix()
	}

	if err := framework.RecordParityRun(ctx, appendonly.ParityRun{
		ID:          recordID,
		Domain:      "issue-content",
		Status:      status,
		DetailsJSON: mustJSON(result),
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		return err
	}

	return printJSON(struct {
		RunID  string                              `json:"runId"`
		Status string                              `json:"status"`
		Result appendonly.IssueContentParityResult `json:"result"`
	}{
		RunID:  recordID,
		Status: status,
		Result: result,
	})
}

func runTagParity(
	ctx context.Context,
	framework *appendonly.FrameworkStore,
	tags *appendonly.TagStore,
	args []string,
) error {
	fs := flag.NewFlagSet("tag-parity", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		detailLimit int
		recordID    string
	)
	fs.IntVar(&detailLimit, "detail-limit", 20, "Maximum number of mismatches to include in the output")
	fs.StringVar(&recordID, "record-id", "", "Parity run id (default: generated)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := tags.CheckParity(ctx, detailLimit)
	if err != nil {
		return err
	}

	status := "pass"
	if result.MissingTags > 0 || result.MismatchedTags > 0 || result.MissingMergedAliases > 0 {
		status = "fail"
	}
	if strings.TrimSpace(recordID) == "" {
		recordID = "parity-" + randomSuffix()
	}

	if err := framework.RecordParityRun(ctx, appendonly.ParityRun{
		ID:          recordID,
		Domain:      "tags",
		Status:      status,
		DetailsJSON: mustJSON(result),
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		return err
	}

	return printJSON(struct {
		RunID  string                     `json:"runId"`
		Status string                     `json:"status"`
		Result appendonly.TagParityResult `json:"result"`
	}{
		RunID:  recordID,
		Status: status,
		Result: result,
	})
}

func randomSuffix() string {
	var randomBytes [4]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	return hex.EncodeToString(randomBytes[:])
}

func usageError() error {
	return fmt.Errorf(
		"usage: go run ./cmd/append-only-migration <status|checkpoint|parity|issue-lifecycle-backfill|issue-lifecycle-parity|issue-enrichment-backfill|issue-enrichment-parity|issue-content-backfill|issue-content-parity|tag-backfill|tag-parity> [flags]",
	)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mustJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal command json payload: %v", err))
	}
	return json.RawMessage(encoded)
}
