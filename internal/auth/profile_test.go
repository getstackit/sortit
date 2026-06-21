package auth

import (
	"context"
	"database/sql"
	"testing"
)

type userProfileFactView struct {
	Sequence    int64
	Login       string
	DisplayName string
	AvatarURL   string
	Email       string
	Source      string
}

func loadUserProfileFacts(t *testing.T, ctx context.Context, db *sql.DB, userID string) []userProfileFactView {
	t.Helper()
	rows, err := db.QueryContext(
		ctx,
		`SELECT sequence, login, display_name, avatar_url, email, source
		 FROM user_profile_facts
		 WHERE user_id = $1
		 ORDER BY sequence ASC`,
		userID,
	)
	if err != nil {
		t.Fatalf("query user profile facts: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	var facts []userProfileFactView
	for rows.Next() {
		var fact userProfileFactView
		if err := rows.Scan(
			&fact.Sequence,
			&fact.Login,
			&fact.DisplayName,
			&fact.AvatarURL,
			&fact.Email,
			&fact.Source,
		); err != nil {
			t.Fatalf("scan user profile fact: %v", err)
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate user profile facts: %v", err)
	}
	return facts
}

func TestUpsertOAuthUserSeedsInitialProfileFact(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t, ctx)
	store := NewStore(db)

	user, err := store.UpsertOAuthUser(ctx, OAuthUser{
		Provider:       "github",
		ProviderUserID: "1",
		Login:          "octocat",
		DisplayName:    "The Octocat",
		AvatarURL:      "https://example.com/a.png",
		Email:          "octocat@example.com",
	})
	if err != nil {
		t.Fatalf("upsert oauth user: %v", err)
	}

	facts := loadUserProfileFacts(t, ctx, db, user.ID)
	if len(facts) != 1 {
		t.Fatalf("expected 1 initial profile fact, got %d (%+v)", len(facts), facts)
	}
	fact := facts[0]
	if fact.Sequence != 1 {
		t.Fatalf("expected sequence 1, got %d", fact.Sequence)
	}
	if fact.Login != "octocat" || fact.Email != "octocat@example.com" || fact.DisplayName != "The Octocat" {
		t.Fatalf("unexpected fact snapshot: %+v", fact)
	}
	if fact.Source != "user_create" {
		t.Fatalf("expected source user_create, got %q", fact.Source)
	}
}

func TestProfileChangeAppendsFactAndNoOpLoginDoesNot(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t, ctx)
	store := NewStore(db)

	base := OAuthUser{
		Provider:       "github",
		ProviderUserID: "1",
		Login:          "octocat",
		DisplayName:    "The Octocat",
		AvatarURL:      "https://example.com/a.png",
		Email:          "octocat@example.com",
	}

	user, err := store.UpsertOAuthUser(ctx, base)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// A login that observes the identical profile must not append a fact.
	if _, err := store.UpsertOAuthUser(ctx, base); err != nil {
		t.Fatalf("no-op upsert: %v", err)
	}
	if facts := loadUserProfileFacts(t, ctx, db, user.ID); len(facts) != 1 {
		t.Fatalf("expected no-op login to leave 1 fact, got %d (%+v)", len(facts), facts)
	}

	// A login with a changed field appends a new fact and updates the projection.
	changed := base
	changed.Email = "octocat@github.com"
	if _, err := store.UpsertOAuthUser(ctx, changed); err != nil {
		t.Fatalf("changed upsert: %v", err)
	}

	facts := loadUserProfileFacts(t, ctx, db, user.ID)
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts after change, got %d (%+v)", len(facts), facts)
	}
	if facts[1].Sequence != 2 || facts[1].Email != "octocat@github.com" || facts[1].Source != "user_login" {
		t.Fatalf("unexpected change fact: %+v", facts[1])
	}

	var email string
	if err := db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = $1`, user.ID).Scan(&email); err != nil {
		t.Fatalf("load projection email: %v", err)
	}
	if email != "octocat@github.com" {
		t.Fatalf("expected projection email updated, got %q", email)
	}
}
