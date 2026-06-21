package appendonly

import (
	"context"
	"database/sql"
	"testing"

	"sortit/internal/issues"
)

func insertSessionRow(t *testing.T, ctx context.Context, store *issues.PostgresStore, id, userID string) {
	t.Helper()
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO sessions (id, user_id, token_hash, expires_at_unix_nano, created_at_unix_nano)
		 VALUES ($1, $2, $3, $4, $5)`,
		id,
		userID,
		"hash-"+id,
		int64(1_900_000_000_000_000_000),
		int64(1_700_000_000_000_000_000),
	); err != nil {
		t.Fatalf("insert session %q: %v", id, err)
	}
}

func countSessionFacts(t *testing.T, ctx context.Context, db *sql.DB, sessionID string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM session_facts WHERE session_id = $1`,
		sessionID,
	).Scan(&count); err != nil {
		t.Fatalf("count session facts for %q: %v", sessionID, err)
	}
	return count
}

func TestSessionBackfillRebuildsFactsAndParityPasses(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	insertUserRow(t, ctx, store, "user_s", "sam", "sam@example.com")
	insertSessionRow(t, ctx, store, "sess_a", "user_s")
	insertSessionRow(t, ctx, store, "sess_b", "user_s")

	sessions := NewSessionStore(store.DB())
	result, err := sessions.BackfillBatch(ctx, "session-backfill-test", 100)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if result.ProcessedSessions != 2 || result.WroteFacts != 2 {
		t.Fatalf("expected 2 processed / 2 facts, got %+v", result)
	}
	if !result.Complete {
		t.Fatalf("expected backfill to be complete")
	}

	parity, err := sessions.CheckParity(ctx, 10)
	if err != nil {
		t.Fatalf("parity: %v", err)
	}
	if parity.MissingFacts != 0 || parity.MismatchedSessions != 0 {
		t.Fatalf("expected parity to pass, got %+v", parity)
	}
	if parity.TotalSessions != 2 || parity.SessionsWithFacts != 2 {
		t.Fatalf("expected 2 sessions with facts, got %+v", parity)
	}
}

func TestSessionBackfillSkipsSessionsThatAlreadyHaveFacts(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	insertUserRow(t, ctx, store, "user_rt", "tina", "tina@example.com")
	insertSessionRow(t, ctx, store, "sess_runtime", "user_rt")

	// Simulate a 'created' fact already written by the runtime CreateSession path.
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO session_facts (id, session_id, user_id, sequence, kind, created_at_unix_nano, source, source_id)
		 VALUES ($1, $2, $3, 1, $4, $5, $6, $7)`,
		"runtime-fact",
		"sess_runtime",
		"user_rt",
		kindCreated,
		int64(1_700_000_000_000_000_000),
		"session_create",
		"sess_runtime:created",
	); err != nil {
		t.Fatalf("seed runtime fact: %v", err)
	}

	sessions := NewSessionStore(store.DB())
	result, err := sessions.BackfillBatch(ctx, "session-skip-test", 100)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if result.WroteFacts != 0 {
		t.Fatalf("expected 0 facts written (already present), got %d", result.WroteFacts)
	}
	if got := countSessionFacts(t, ctx, store.DB(), "sess_runtime"); got != 1 {
		t.Fatalf("expected exactly 1 fact (no duplicate), got %d", got)
	}
}

func TestSessionParityDetectsMissingAndInvalidatedActive(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	insertUserRow(t, ctx, store, "user_p", "pat", "pat@example.com")
	insertSessionRow(t, ctx, store, "sess_orphan", "user_p")
	insertSessionRow(t, ctx, store, "sess_zombie", "user_p")

	// sess_zombie is still in the projection but carries an 'invalidated' fact —
	// it should have been removed on logout, so parity must flag it.
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO session_facts (id, session_id, user_id, sequence, kind, created_at_unix_nano, source, source_id)
		 VALUES ($1, $2, $3, 1, $4, $5, $6, $7), ($8, $2, $3, 2, $9, $10, $11, $12)`,
		"zombie-created", "sess_zombie", "user_p", kindCreated, int64(1_700_000_000_000_000_000), "session_create", "sess_zombie:created",
		"zombie-invalidated", sessionKindInvalidated, int64(1_700_000_500_000_000_000), "session_invalidate", "sess_zombie:invalidated",
	); err != nil {
		t.Fatalf("seed zombie facts: %v", err)
	}

	sessions := NewSessionStore(store.DB())
	parity, err := sessions.CheckParity(ctx, 10)
	if err != nil {
		t.Fatalf("parity: %v", err)
	}
	if parity.MissingFacts != 1 {
		t.Fatalf("expected 1 missing fact (orphan), got %+v", parity)
	}
	if parity.MismatchedSessions != 2 {
		t.Fatalf("expected 2 mismatched sessions (orphan + zombie), got %+v", parity)
	}
}
