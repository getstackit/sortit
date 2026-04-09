package appendonly

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"sortit/internal/issues"
	"sortit/internal/testpostgres"
)

func TestLifecycleBackfillRebuildsProjectionFromLegacyPosts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newLifecycleTestStore(t, ctx)
	lifecycle := NewLifecycleStore(store.DB())

	issue := issues.BuildNewIssue("issue-lifecycle-000001", issues.CreateInput{
		Raw:       "Export fails after assigning owner",
		CreatedBy: "Casey",
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	detail, err := store.GetIssueDetail(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get issue detail: %v", err)
	}

	assignPost := issues.NewDiscussionPost(
		issue.ID,
		detail.Discussion,
		issues.AssignIssuePost("Jordan"),
		"Casey",
		"assigned",
	)
	if err := store.SaveIssuePost(ctx, assignPost); err != nil {
		t.Fatalf("save assign post: %v", err)
	}
	assignee := "Jordan" //nolint:goconst
	if err := store.UpdateIssueFields(ctx, issue.ID, issues.IssueFieldUpdate{
		AssignedTo: &assignee,
	}); err != nil {
		t.Fatalf("assign issue: %v", err)
	}

	detail, err = store.GetIssueDetail(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get issue detail after assign: %v", err)
	}

	reason := "duplicate"
	reasonNote := "tracked elsewhere"
	closePost := issues.NewDiscussionPost(
		issue.ID,
		detail.Discussion,
		issues.CloseIssuePost("Jordan", reason, reasonNote),
		"Jordan",
		"closed",
	)
	if err := store.SaveIssuePost(ctx, closePost); err != nil {
		t.Fatalf("save close post: %v", err)
	}
	closedStatus := issues.StatusClosed
	closedBy := closePost.CreatedBy
	closedAt := closePost.CreatedAt
	if err := store.UpdateIssueFields(ctx, issue.ID, issues.IssueFieldUpdate{
		Status:           &closedStatus,
		ClosedAt:         &closedAt,
		ClosedBy:         &closedBy,
		ClosedReason:     &reason,
		ClosedReasonNote: &reasonNote,
	}); err != nil {
		t.Fatalf("close issue: %v", err)
	}

	detail, err = store.GetIssueDetail(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get issue detail after close: %v", err)
	}

	reopenPost := issues.NewDiscussionPost(
		issue.ID,
		detail.Discussion,
		issues.ReopenIssuePost(),
		"",
		"reopened",
	)
	if err := store.SaveIssuePost(ctx, reopenPost); err != nil {
		t.Fatalf("save reopen post: %v", err)
	}
	openStatus := issues.StatusOpen
	if err := store.UpdateIssueFields(ctx, issue.ID, issues.IssueFieldUpdate{
		Status: &openStatus,
	}); err != nil {
		t.Fatalf("reopen issue: %v", err)
	}

	result, err := lifecycle.BackfillBatch(ctx, store, "issue-lifecycle-backfill-test", 10)
	if err != nil {
		t.Fatalf("backfill lifecycle batch: %v", err)
	}
	if result.ProcessedIssues != 1 {
		t.Fatalf("expected 1 processed issue, got %d", result.ProcessedIssues)
	}
	if result.WroteProjections != 1 {
		t.Fatalf("expected 1 projection write, got %d", result.WroteProjections)
	}

	parity, err := lifecycle.CheckParity(ctx, store, 10)
	if err != nil {
		t.Fatalf("check lifecycle parity: %v", err)
	}
	if parity.MismatchedIssues != 0 || parity.MissingProjections != 0 {
		t.Fatalf("expected parity match, got %#v", parity)
	}

	facts := loadLifecycleFacts(t, ctx, store.DB(), issue.ID)
	if len(facts) != 4 {
		t.Fatalf("expected 4 lifecycle facts, got %d", len(facts))
	}
	expectedKinds := []string{"created", "assigned", "closed", "reopened"}
	for i, kind := range expectedKinds {
		if facts[i].Kind != kind {
			t.Fatalf("expected fact %d kind %q, got %q", i, kind, facts[i].Kind)
		}
		if facts[i].Inferred {
			t.Fatalf("expected fact %d to be observed, got inferred", i)
		}
	}

	projection := loadLifecycleProjection(t, ctx, store.DB(), issue.ID)
	if projection.Status != issues.StatusOpen {
		t.Fatalf("expected open projection status, got %q", projection.Status)
	}
	if projection.AssignedTo != "Jordan" {
		t.Fatalf("expected assigned projection to Jordan, got %q", projection.AssignedTo)
	}
	if projection.ClosedAt != nil {
		t.Fatalf("expected reopened projection to clear closed_at, got %v", projection.ClosedAt)
	}
}

func TestLifecycleBackfillAddsInferredFactForMissingAssignmentHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newLifecycleTestStore(t, ctx)
	lifecycle := NewLifecycleStore(store.DB())

	issue := issues.BuildNewIssue("issue-lifecycle-000002", issues.CreateInput{
		Raw:       "Needs owner but no post history",
		CreatedBy: "Casey",
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	assignee := "Jordan" //nolint:goconst
	if err := store.UpdateIssueFields(ctx, issue.ID, issues.IssueFieldUpdate{
		AssignedTo: &assignee,
	}); err != nil {
		t.Fatalf("assign issue without post: %v", err)
	}

	if _, err := lifecycle.BackfillBatch(ctx, store, "issue-lifecycle-backfill-test", 10); err != nil {
		t.Fatalf("backfill lifecycle batch: %v", err)
	}

	parity, err := lifecycle.CheckParity(ctx, store, 10)
	if err != nil {
		t.Fatalf("check lifecycle parity: %v", err)
	}
	if parity.MismatchedIssues != 0 || parity.MissingProjections != 0 {
		t.Fatalf("expected parity match, got %#v", parity)
	}

	facts := loadLifecycleFacts(t, ctx, store.DB(), issue.ID)
	if len(facts) != 2 {
		t.Fatalf("expected 2 lifecycle facts, got %d", len(facts))
	}
	if facts[1].Kind != "assigned" {
		t.Fatalf("expected second fact to be assigned, got %q", facts[1].Kind)
	}
	if !facts[1].Inferred {
		t.Fatalf("expected missing assignment history to produce an inferred fact")
	}

	var payload lifecycleAssignedPayload
	if err := json.Unmarshal(facts[1].Payload, &payload); err != nil {
		t.Fatalf("decode assigned payload: %v", err)
	}
	if payload.AssignedTo != "Jordan" {
		t.Fatalf("expected inferred assignee Jordan, got %q", payload.AssignedTo)
	}
}

type lifecycleFactRow struct {
	Kind     string
	Inferred bool
	Payload  json.RawMessage
}

func newLifecycleTestStore(t *testing.T, ctx context.Context) *issues.PostgresStore {
	t.Helper()

	harness, err := testpostgres.Start(ctx, "sortit_appendonly_lifecycle_test")
	if err != nil {
		t.Fatalf("start postgres harness: %v", err)
	}
	t.Cleanup(func() {
		if err := harness.Terminate(ctx); err != nil {
			t.Fatalf("terminate postgres harness: %v", err)
		}
	})

	databaseURL := harness.Acquire(t, func(ctx context.Context, databaseURL string) error {
		store, err := issues.OpenPostgresStore(ctx, databaseURL)
		if err != nil {
			return err
		}
		return store.Close()
	})

	store, err := issues.OpenPostgresStore(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close postgres store: %v", err)
		}
	})
	return store
}

func loadLifecycleFacts(t *testing.T, ctx context.Context, db *sql.DB, issueID string) []lifecycleFactRow {
	t.Helper()

	rows, err := db.QueryContext(
		ctx,
		`SELECT kind, inferred, payload_json
		 FROM issue_lifecycle_facts
		 WHERE issue_id = $1
		 ORDER BY sequence ASC, created_at_unix_nano ASC, id ASC`,
		issueID,
	)
	if err != nil {
		t.Fatalf("query lifecycle facts: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	var facts []lifecycleFactRow
	for rows.Next() {
		var fact lifecycleFactRow
		if err := rows.Scan(&fact.Kind, &fact.Inferred, &fact.Payload); err != nil {
			t.Fatalf("scan lifecycle fact: %v", err)
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate lifecycle facts: %v", err)
	}
	return facts
}

func loadLifecycleProjection(t *testing.T, ctx context.Context, db *sql.DB, issueID string) LifecycleProjection {
	t.Helper()

	store := NewLifecycleStore(db)
	projections, err := store.loadAllProjections(ctx)
	if err != nil {
		t.Fatalf("load lifecycle projections: %v", err)
	}
	projection, ok := projections[issueID]
	if !ok {
		t.Fatalf("missing lifecycle projection for %q", issueID)
	}
	return projection
}
