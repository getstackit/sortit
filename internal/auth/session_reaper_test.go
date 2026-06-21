package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func countSessionFactsByReason(t *testing.T, ctx context.Context, db *sql.DB, reason string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM session_facts WHERE kind = $1 AND reason = $2`,
		sessionFactKindInvalidated,
		reason,
	).Scan(&count); err != nil {
		t.Fatalf("count session facts by reason %q: %v", reason, err)
	}
	return count
}

func TestReapExpiredSessionsWritesExpiredFactAndPrunesProjection(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t, ctx)
	store := NewStore(db)
	user := newTestUser(t, ctx, store)

	if _, err := store.CreateSession(ctx, user.ID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("create expired session: %v", err)
	}

	res, err := store.ReapExpiredSessions(ctx, time.Now().UTC(), 100)
	if err != nil {
		t.Fatalf("reap expired sessions: %v", err)
	}
	if res.Scanned != 1 || res.Reaped != 1 {
		t.Fatalf("expected scanned=1 reaped=1, got %+v", res)
	}

	// Projection row pruned; history preserved as facts.
	if got := countSessions(t, ctx, db, user.ID); got != 0 {
		t.Fatalf("expected projection row reaped, got %d", got)
	}
	facts := loadSessionFacts(t, ctx, db, user.ID)
	if len(facts) != 2 {
		t.Fatalf("expected created + expired facts, got %d (%+v)", len(facts), facts)
	}
	expired := facts[1]
	if expired.Kind != sessionFactKindInvalidated ||
		expired.Reason != sessionReasonExpired ||
		expired.Sequence != 2 ||
		expired.Source != "session_expire" {
		t.Fatalf("unexpected expired fact: %+v", expired)
	}
}

func TestReapExpiredSessionsLeavesActiveSessionsAlone(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t, ctx)
	store := NewStore(db)
	user := newTestUser(t, ctx, store)

	if _, err := store.CreateSession(ctx, user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create active session: %v", err)
	}

	res, err := store.ReapExpiredSessions(ctx, time.Now().UTC(), 100)
	if err != nil {
		t.Fatalf("reap expired sessions: %v", err)
	}
	if res.Scanned != 0 || res.Reaped != 0 {
		t.Fatalf("expected nothing reaped, got %+v", res)
	}
	if got := countSessions(t, ctx, db, user.ID); got != 1 {
		t.Fatalf("expected active session retained, got %d", got)
	}
	if facts := loadSessionFacts(t, ctx, db, user.ID); len(facts) != 1 {
		t.Fatalf("expected only the created fact, got %d (%+v)", len(facts), facts)
	}
}

func TestReapExpiredSessionsIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t, ctx)
	store := NewStore(db)
	user := newTestUser(t, ctx, store)

	if _, err := store.CreateSession(ctx, user.ID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("create expired session: %v", err)
	}

	if _, err := store.ReapExpiredSessions(ctx, time.Now().UTC(), 100); err != nil {
		t.Fatalf("first reap: %v", err)
	}
	// A second sweep finds nothing to do and writes no duplicate fact.
	res, err := store.ReapExpiredSessions(ctx, time.Now().UTC(), 100)
	if err != nil {
		t.Fatalf("second reap: %v", err)
	}
	if res.Scanned != 0 || res.Reaped != 0 {
		t.Fatalf("expected second sweep to be a no-op, got %+v", res)
	}
	if facts := loadSessionFacts(t, ctx, db, user.ID); len(facts) != 2 {
		t.Fatalf("expected exactly created + one expired fact, got %d (%+v)", len(facts), facts)
	}
}

func TestSessionReaperSweepDrainsAcrossBatches(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t, ctx)
	store := NewStore(db)
	user := newTestUser(t, ctx, store)

	const expiredCount = 3
	for i := range expiredCount {
		if _, err := store.CreateSession(ctx, user.ID, time.Now().Add(-time.Hour)); err != nil {
			t.Fatalf("create expired session %d: %v", i, err)
		}
	}

	// BatchSize=1 forces the sweep to loop until a short batch signals it is drained.
	reaper := &SessionReaper{Store: store, BatchSize: 1}
	total, err := reaper.sweep(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if total != expiredCount {
		t.Fatalf("expected sweep to reap %d sessions, got %d", expiredCount, total)
	}
	if got := countSessions(t, ctx, db, user.ID); got != 0 {
		t.Fatalf("expected all expired projection rows reaped, got %d", got)
	}
	if got := countSessionFactsByReason(t, ctx, db, sessionReasonExpired); got != expiredCount {
		t.Fatalf("expected %d expired facts, got %d", expiredCount, got)
	}
}
