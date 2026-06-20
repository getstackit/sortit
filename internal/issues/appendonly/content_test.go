package appendonly

import (
	"context"
	"testing"
	"time"
)

func TestIssueContentBackfillRebuildsProjectionFromLegacyIssues(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	content := NewIssueContentStore(store.DB())

	createdAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO issues (
		     id,
		     raw,
		     tags_json,
		     created_by,
		     created_at_unix_nano,
		     status,
		     tag_scores_json,
		     assigned_to,
		     embedding_vector
		 ) VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7::jsonb, $8, $9::vector)`,
		"issue-content-backfill-000001",
		"Legacy canonical content",
		`["bug"]`,
		"Casey",
		createdAt.UnixNano(),
		"open",
		`[{"tag":"bug","relevance":0.9}]`,
		"",
		"[0.2,0.8]",
	); err != nil {
		t.Fatalf("insert legacy issue row: %v", err)
	}

	result, err := content.BackfillBatch(ctx, "issue-content-backfill-test", 100)
	if err != nil {
		t.Fatalf("backfill issue content: %v", err)
	}
	if result.ProcessedIssues != 1 || result.WroteProjections != 1 {
		t.Fatalf("unexpected issue content backfill result: %#v", result)
	}

	parity, err := content.CheckParity(ctx, 10)
	if err != nil {
		t.Fatalf("check issue content parity: %v", err)
	}
	if parity.MissingProjections != 0 || parity.MismatchedIssues != 0 {
		t.Fatalf("expected issue content parity match, got %#v", parity)
	}
}
