package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func newTestUser(t *testing.T, ctx context.Context, store *Store) User {
	t.Helper()
	user, err := store.UpsertOAuthUser(ctx, OAuthUser{
		Provider:       "github",
		ProviderUserID: "12345",
		Login:          "octocat",
		DisplayName:    "The Octocat",
		AvatarURL:      "https://example.com/avatar.png",
		Email:          "octocat@example.com",
	})
	if err != nil {
		t.Fatalf("upsert oauth user: %v", err)
	}
	return user
}

type apiTokenFactView struct {
	Sequence  int64
	Kind      string
	UserID    string
	Source    string
	CreatedBy string
}

func loadAPITokenFacts(t *testing.T, ctx context.Context, db *sql.DB, tokenID string) []apiTokenFactView {
	t.Helper()
	rows, err := db.QueryContext(
		ctx,
		`SELECT sequence, kind, user_id, source, created_by
		 FROM api_token_facts
		 WHERE token_id = $1
		 ORDER BY sequence ASC`,
		tokenID,
	)
	if err != nil {
		t.Fatalf("query api token facts: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	var facts []apiTokenFactView
	for rows.Next() {
		var fact apiTokenFactView
		if err := rows.Scan(&fact.Sequence, &fact.Kind, &fact.UserID, &fact.Source, &fact.CreatedBy); err != nil {
			t.Fatalf("scan api token fact: %v", err)
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate api token facts: %v", err)
	}
	return facts
}

func TestCreateAPITokenWritesCreatedFact(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t, ctx)
	store := NewStore(db)
	user := newTestUser(t, ctx, store)

	token, _, err := store.CreateAPIToken(ctx, user.ID, "ci")
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}

	facts := loadAPITokenFacts(t, ctx, db, token.ID)
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact after create, got %d (%+v)", len(facts), facts)
	}
	created := facts[0]
	if created.Kind != apiTokenFactKindCreated {
		t.Fatalf("expected created fact, got kind %q", created.Kind)
	}
	if created.Sequence != 1 {
		t.Fatalf("expected sequence 1, got %d", created.Sequence)
	}
	if created.UserID != user.ID {
		t.Fatalf("expected user id %q, got %q", user.ID, created.UserID)
	}
	if created.CreatedBy != user.ID {
		t.Fatalf("expected created_by %q, got %q", user.ID, created.CreatedBy)
	}
	if created.Source != "api_token_create" {
		t.Fatalf("expected source api_token_create, got %q", created.Source)
	}
}

func TestRevokeAPITokenWritesRevokedFactAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t, ctx)
	store := NewStore(db)
	user := newTestUser(t, ctx, store)

	token, _, err := store.CreateAPIToken(ctx, user.ID, "ci")
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}

	if err := store.RevokeAPIToken(ctx, user.ID, token.ID); err != nil {
		t.Fatalf("revoke api token: %v", err)
	}

	facts := loadAPITokenFacts(t, ctx, db, token.ID)
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts after revoke, got %d (%+v)", len(facts), facts)
	}
	if facts[1].Kind != apiTokenFactKindRevoked {
		t.Fatalf("expected revoked fact second, got kind %q", facts[1].Kind)
	}
	if facts[1].Sequence != 2 {
		t.Fatalf("expected revoked sequence 2, got %d", facts[1].Sequence)
	}

	// The projection records the revocation.
	var revokedAt int64
	if err := db.QueryRowContext(
		ctx,
		`SELECT revoked_at_unix_nano FROM api_tokens WHERE id = $1`,
		token.ID,
	).Scan(&revokedAt); err != nil {
		t.Fatalf("load projection revoked_at: %v", err)
	}
	if revokedAt <= 0 {
		t.Fatalf("expected projection revoked_at to be set, got %d", revokedAt)
	}

	// Re-revoking is a no-op: it must not produce a spurious 'revoked' fact.
	if err := store.RevokeAPIToken(ctx, user.ID, token.ID); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("expected ErrTokenNotFound re-revoking, got %v", err)
	}
	if facts := loadAPITokenFacts(t, ctx, db, token.ID); len(facts) != 2 {
		t.Fatalf("expected still 2 facts after re-revoke, got %d (%+v)", len(facts), facts)
	}
}
