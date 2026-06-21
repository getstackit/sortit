package appendonly

import (
	"context"
	"database/sql"
	"testing"

	"sortit/internal/issues"
)

func insertUserRow(t *testing.T, ctx context.Context, store *issues.PostgresStore, id, login, email string) {
	t.Helper()
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO users (id, login, display_name, avatar_url, email, created_at_unix_nano, updated_at_unix_nano)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		id,
		login,
		login+" display",
		"https://example.com/"+id+".png",
		email,
		int64(1_700_000_000_000_000_000),
	); err != nil {
		t.Fatalf("insert user %q: %v", id, err)
	}
}

func countUserProfileFacts(t *testing.T, ctx context.Context, db *sql.DB, userID string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM user_profile_facts WHERE user_id = $1`,
		userID,
	).Scan(&count); err != nil {
		t.Fatalf("count user profile facts for %q: %v", userID, err)
	}
	return count
}

func TestUserProfileBackfillRebuildsFactsAndParityPasses(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	insertUserRow(t, ctx, store, "user_a", "alice", "alice@example.com")
	insertUserRow(t, ctx, store, "user_b", "bob", "bob@example.com")

	profiles := NewUserProfileStore(store.DB())
	result, err := profiles.BackfillBatch(ctx, "user-profile-backfill-test", 100)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if result.ProcessedUsers != 2 || result.WroteFacts != 2 {
		t.Fatalf("expected 2 processed / 2 facts, got %+v", result)
	}
	if !result.Complete {
		t.Fatalf("expected backfill to be complete")
	}

	parity, err := profiles.CheckParity(ctx, 10)
	if err != nil {
		t.Fatalf("parity: %v", err)
	}
	if parity.MissingFacts != 0 || parity.MismatchedUsers != 0 {
		t.Fatalf("expected parity to pass, got %+v", parity)
	}
	if parity.TotalUsers != 2 || parity.UsersWithFacts != 2 {
		t.Fatalf("expected 2 users with facts, got %+v", parity)
	}
}

func TestUserProfileBackfillSkipsUsersThatAlreadyHaveFacts(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	insertUserRow(t, ctx, store, "user_runtime", "carol", "carol@example.com")

	// Simulate a profile fact already written by the runtime login path.
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO user_profile_facts (id, user_id, sequence, login, display_name, avatar_url, email, observed_at_unix_nano, source, source_id)
		 VALUES ($1, $2, 1, $3, $4, $5, $6, $7, $8, $9)`,
		"runtime-fact",
		"user_runtime",
		"carol",
		"carol display",
		"https://example.com/user_runtime.png",
		"carol@example.com",
		int64(1_700_000_000_000_000_000),
		"user_create",
		"user_runtime:created",
	); err != nil {
		t.Fatalf("seed runtime fact: %v", err)
	}

	profiles := NewUserProfileStore(store.DB())
	result, err := profiles.BackfillBatch(ctx, "user-profile-skip-test", 100)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if result.WroteFacts != 0 {
		t.Fatalf("expected 0 facts written (already present), got %d", result.WroteFacts)
	}
	if got := countUserProfileFacts(t, ctx, store.DB(), "user_runtime"); got != 1 {
		t.Fatalf("expected exactly 1 fact (no duplicate), got %d", got)
	}
}

func TestUserProfileParityDetectsMissingAndMismatchedFacts(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	insertUserRow(t, ctx, store, "user_orphan", "dave", "dave@example.com")
	insertUserRow(t, ctx, store, "user_drift", "erin", "erin@example.com")

	// user_drift has a fact, but its email no longer matches the projection.
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO user_profile_facts (id, user_id, sequence, login, display_name, avatar_url, email, observed_at_unix_nano, source, source_id)
		 VALUES ($1, $2, 1, $3, $4, $5, $6, $7, $8, $9)`,
		"drift-fact",
		"user_drift",
		"erin",
		"erin display",
		"https://example.com/user_drift.png",
		"stale@example.com",
		int64(1_700_000_000_000_000_000),
		"user_create",
		"user_drift:created",
	); err != nil {
		t.Fatalf("seed drift fact: %v", err)
	}

	profiles := NewUserProfileStore(store.DB())
	parity, err := profiles.CheckParity(ctx, 10)
	if err != nil {
		t.Fatalf("parity: %v", err)
	}
	if parity.MissingFacts != 1 {
		t.Fatalf("expected 1 missing fact (orphan), got %+v", parity)
	}
	if parity.MismatchedUsers != 2 {
		t.Fatalf("expected 2 mismatched users (orphan + drift), got %+v", parity)
	}
}
