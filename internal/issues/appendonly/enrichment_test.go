package appendonly

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sortit/internal/issues"
)

func TestEnrichmentBackfillRebuildsProjectionFromLegacyState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newLifecycleTestStore(t, ctx)
	enrichment := NewEnrichmentStore(store.DB())

	issue := issues.BuildNewIssue("issue-enrichment-000001", issues.CreateInput{
		Raw:       "Need enrichment history",
		CreatedBy: "Casey",
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	pendingStatus := issues.EnrichmentStatusPending
	emptyError := ""
	targetSequence := 2
	if err := store.UpdateIssueFields(ctx, issue.ID, issues.IssueFieldUpdate{
		EnrichmentStatus:         &pendingStatus,
		EnrichmentError:          &emptyError,
		EnrichmentTargetSequence: &targetSequence,
	}); err != nil {
		t.Fatalf("set pending enrichment state: %v", err)
	}
	if err := store.EnqueueIssueEnrichment(ctx, issue.ID, targetSequence); err != nil {
		t.Fatalf("enqueue enrichment job: %v", err)
	}
	job, ok, err := store.ClaimNextIssueEnrichment(ctx, time.Minute)
	if err != nil {
		t.Fatalf("claim enrichment job: %v", err)
	}
	if !ok {
		t.Fatal("expected enrichment job to be claimable")
	}
	if job.IssueID != issue.ID || job.TargetSequence != targetSequence || job.AttemptCount != 1 {
		t.Fatalf("unexpected claimed job %#v", job)
	}

	result, err := enrichment.BackfillBatch(ctx, "issue-enrichment-backfill-test", 10)
	if err != nil {
		t.Fatalf("backfill enrichment batch: %v", err)
	}
	if result.ProcessedIssues != 1 {
		t.Fatalf("expected 1 processed issue, got %d", result.ProcessedIssues)
	}

	parity, err := enrichment.CheckParity(ctx, 10)
	if err != nil {
		t.Fatalf("check enrichment parity: %v", err)
	}
	if parity.MismatchedIssues != 0 || parity.MissingProjections != 0 {
		t.Fatalf("expected enrichment parity match, got %#v", parity)
	}

	var (
		status           string
		errorText        string
		projectionTarget int
		attemptCount     int
		leaseExpiresAt   int64
	)
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT status, error, target_sequence, attempt_count, lease_expires_at_unix_nano
		 FROM issue_enrichment_projections
		 WHERE issue_id = $1`,
		issue.ID,
	).Scan(&status, &errorText, &projectionTarget, &attemptCount, &leaseExpiresAt); err != nil {
		t.Fatalf("read enrichment projection: %v", err)
	}
	if status != string(issues.EnrichmentStatusPending) {
		t.Fatalf("expected pending enrichment projection, got %q", status)
	}
	if errorText != "" {
		t.Fatalf("expected empty enrichment error, got %q", errorText)
	}
	if projectionTarget != targetSequence || attemptCount != 1 || leaseExpiresAt == 0 {
		t.Fatalf("unexpected enrichment projection values: target=%d attempt=%d lease=%d", projectionTarget, attemptCount, leaseExpiresAt)
	}

	var (
		kind        string
		eventTarget int
		payload     json.RawMessage
	)
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT kind, target_sequence, payload_json
		 FROM issue_enrichment_events
		 WHERE issue_id = $1`,
		issue.ID,
	).Scan(&kind, &eventTarget, &payload); err != nil {
		t.Fatalf("read enrichment event: %v", err)
	}
	if kind != "observed_state" || eventTarget != targetSequence {
		t.Fatalf("unexpected enrichment event kind=%q target=%d", kind, eventTarget)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode enrichment payload: %v", err)
	}
	if decoded["status"] != string(issues.EnrichmentStatusPending) {
		t.Fatalf("expected observed status pending, got %#v", decoded["status"])
	}
}
