package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

type sessionFactView struct {
	Sequence int64
	Kind     string
	Reason   string
	Source   string
}

func loadSessionFacts(t *testing.T, ctx context.Context, db *sql.DB, userID string) []sessionFactView {
	t.Helper()
	rows, err := db.QueryContext(
		ctx,
		`SELECT sequence, kind, reason, source
		 FROM session_facts
		 WHERE user_id = $1
		 ORDER BY sequence ASC`,
		userID,
	)
	if err != nil {
		t.Fatalf("query session facts: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	var facts []sessionFactView
	for rows.Next() {
		var fact sessionFactView
		if err := rows.Scan(&fact.Sequence, &fact.Kind, &fact.Reason, &fact.Source); err != nil {
			t.Fatalf("scan session fact: %v", err)
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate session facts: %v", err)
	}
	return facts
}

func countSessions(t *testing.T, ctx context.Context, db *sql.DB, userID string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sessions WHERE user_id = $1`,
		userID,
	).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return count
}

func TestCreateSessionWritesCreatedFact(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t, ctx)
	store := NewStore(db)
	user := newTestUser(t, ctx, store)

	if _, err := store.CreateSession(ctx, user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if got := countSessions(t, ctx, db, user.ID); got != 1 {
		t.Fatalf("expected 1 active session projection row, got %d", got)
	}

	facts := loadSessionFacts(t, ctx, db, user.ID)
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact after create, got %d (%+v)", len(facts), facts)
	}
	if facts[0].Kind != sessionFactKindCreated || facts[0].Sequence != 1 || facts[0].Source != "session_create" {
		t.Fatalf("unexpected created fact: %+v", facts[0])
	}
}

func TestDeleteSessionInvalidatesAndRemovesProjection(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t, ctx)
	store := NewStore(db)
	user := newTestUser(t, ctx, store)

	rawToken, err := store.CreateSession(ctx, user.ID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := store.DeleteSession(ctx, rawToken); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	// The active-session projection row is gone; history is preserved as facts.
	if got := countSessions(t, ctx, db, user.ID); got != 0 {
		t.Fatalf("expected projection row removed, got %d", got)
	}
	facts := loadSessionFacts(t, ctx, db, user.ID)
	if len(facts) != 2 {
		t.Fatalf("expected created + invalidated facts, got %d (%+v)", len(facts), facts)
	}
	if facts[1].Kind != sessionFactKindInvalidated || facts[1].Reason != sessionReasonLogout || facts[1].Sequence != 2 {
		t.Fatalf("unexpected invalidated fact: %+v", facts[1])
	}

	// Deleting an unknown/already-invalidated token is a no-op (no fact, no error).
	if err := store.DeleteSession(ctx, rawToken); err != nil {
		t.Fatalf("re-delete session: %v", err)
	}
	if facts := loadSessionFacts(t, ctx, db, user.ID); len(facts) != 2 {
		t.Fatalf("expected still 2 facts after re-delete, got %d (%+v)", len(facts), facts)
	}
}
