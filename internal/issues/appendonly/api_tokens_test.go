package appendonly

import (
	"context"
	"database/sql"
	"testing"

	"sortit/internal/issues"
)

func insertAPITokenTestUser(t *testing.T, ctx context.Context, store *issues.PostgresStore, userID string) {
	t.Helper()
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO users (id, login, display_name, avatar_url, email, created_at_unix_nano, updated_at_unix_nano)
		 VALUES ($1, $2, $3, '', $4, $5, $5)`,
		userID,
		userID,
		userID,
		userID+"@example.com",
		int64(1_700_000_000_000_000_000),
	); err != nil {
		t.Fatalf("insert user %q: %v", userID, err)
	}
}

func insertLegacyAPIToken(t *testing.T, ctx context.Context, store *issues.PostgresStore, id, userID string, revokedAt int64) {
	t.Helper()
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO api_tokens (id, user_id, token_hash, token_prefix, name, created_at_unix_nano, revoked_at_unix_nano)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id,
		userID,
		"hash-"+id,
		"prefix-"+id,
		id,
		int64(1_700_000_000_000_000_000),
		revokedAt,
	); err != nil {
		t.Fatalf("insert api token %q: %v", id, err)
	}
}

func countAPITokenFacts(t *testing.T, ctx context.Context, db *sql.DB, tokenID string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM api_token_facts WHERE token_id = $1`,
		tokenID,
	).Scan(&count); err != nil {
		t.Fatalf("count api token facts for %q: %v", tokenID, err)
	}
	return count
}

func TestAPITokenBackfillRebuildsFactsAndParityPasses(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	insertAPITokenTestUser(t, ctx, store, "user_a")
	insertLegacyAPIToken(t, ctx, store, "tok_active", "user_a", 0)
	insertLegacyAPIToken(t, ctx, store, "tok_revoked", "user_a", int64(1_700_000_500_000_000_000))

	apiTokens := NewAPITokenStore(store.DB())
	result, err := apiTokens.BackfillBatch(ctx, "api-token-backfill-test", 100)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if result.ProcessedTokens != 2 {
		t.Fatalf("expected 2 processed tokens, got %d", result.ProcessedTokens)
	}
	if result.WroteFacts != 3 {
		t.Fatalf("expected 3 facts (created+created+revoked), got %d", result.WroteFacts)
	}
	if !result.Complete {
		t.Fatalf("expected backfill to be complete")
	}

	if got := countAPITokenFacts(t, ctx, store.DB(), "tok_active"); got != 1 {
		t.Fatalf("expected 1 fact for active token, got %d", got)
	}
	if got := countAPITokenFacts(t, ctx, store.DB(), "tok_revoked"); got != 2 {
		t.Fatalf("expected 2 facts for revoked token, got %d", got)
	}

	parity, err := apiTokens.CheckParity(ctx, 10)
	if err != nil {
		t.Fatalf("parity: %v", err)
	}
	if parity.MissingFacts != 0 || parity.MismatchedTokens != 0 {
		t.Fatalf("expected parity to pass, got %+v", parity)
	}
	if parity.TotalTokens != 2 || parity.TokensWithFacts != 2 {
		t.Fatalf("expected 2 tokens with facts, got %+v", parity)
	}
}

func TestAPITokenBackfillSkipsTokensThatAlreadyHaveFacts(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	insertAPITokenTestUser(t, ctx, store, "user_b")
	insertLegacyAPIToken(t, ctx, store, "tok_runtime", "user_b", 0)

	// Simulate a token whose 'created' fact was already written by the runtime
	// path (sequence 1). The backfill must not duplicate it or collide on the
	// unique (token_id, sequence) index.
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO api_token_facts (id, token_id, user_id, sequence, kind, created_at_unix_nano, source, source_id)
		 VALUES ($1, $2, $3, 1, $4, $5, $6, $7)`,
		"runtime-fact",
		"tok_runtime",
		"user_b",
		apiTokenKindCreated,
		int64(1_700_000_000_000_000_000),
		"api_token_create",
		"tok_runtime:created",
	); err != nil {
		t.Fatalf("seed runtime fact: %v", err)
	}

	apiTokens := NewAPITokenStore(store.DB())
	result, err := apiTokens.BackfillBatch(ctx, "api-token-backfill-skip-test", 100)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if result.WroteFacts != 0 {
		t.Fatalf("expected 0 facts written (already present), got %d", result.WroteFacts)
	}
	if got := countAPITokenFacts(t, ctx, store.DB(), "tok_runtime"); got != 1 {
		t.Fatalf("expected exactly 1 fact (no duplicate), got %d", got)
	}

	parity, err := apiTokens.CheckParity(ctx, 10)
	if err != nil {
		t.Fatalf("parity: %v", err)
	}
	if parity.MissingFacts != 0 || parity.MismatchedTokens != 0 {
		t.Fatalf("expected parity to pass, got %+v", parity)
	}
}

func TestAPITokenParityDetectsMissingCreatedFact(t *testing.T) {
	ctx := context.Background()
	store := newAppendonlyTestStore(t, ctx)
	insertAPITokenTestUser(t, ctx, store, "user_c")
	insertLegacyAPIToken(t, ctx, store, "tok_orphan", "user_c", 0)

	apiTokens := NewAPITokenStore(store.DB())
	parity, err := apiTokens.CheckParity(ctx, 10)
	if err != nil {
		t.Fatalf("parity: %v", err)
	}
	if parity.MissingFacts != 1 || parity.MismatchedTokens != 1 {
		t.Fatalf("expected 1 missing-fact mismatch, got %+v", parity)
	}
}
