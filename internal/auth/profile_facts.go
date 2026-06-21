package auth

import (
	"context"
	"fmt"
	"time"
)

// profileSnapshot is the set of mutable profile fields observed on login. The
// users table is the latest-profile projection; user_profile_facts is the
// append-only history of these snapshots.
type profileSnapshot struct {
	Login       string
	DisplayName string
	AvatarURL   string
	Email       string
}

func (p profileSnapshot) equal(other profileSnapshot) bool {
	return p.Login == other.Login &&
		p.DisplayName == other.DisplayName &&
		p.AvatarURL == other.AvatarURL &&
		p.Email == other.Email
}

func loadCurrentProfile(ctx context.Context, db factDBTX, userID string) (profileSnapshot, error) {
	var snapshot profileSnapshot
	if err := db.QueryRowContext(
		ctx,
		`SELECT login, display_name, avatar_url, email FROM users WHERE id = $1`,
		userID,
	).Scan(&snapshot.Login, &snapshot.DisplayName, &snapshot.AvatarURL, &snapshot.Email); err != nil {
		return profileSnapshot{}, fmt.Errorf("load current profile for %q: %w", userID, err)
	}
	return snapshot, nil
}

func nextUserProfileFactSequence(ctx context.Context, db factDBTX, userID string) (int64, error) {
	var sequence int64
	if err := db.QueryRowContext(
		ctx,
		`SELECT COALESCE(MAX(sequence), 0) + 1 FROM user_profile_facts WHERE user_id = $1`,
		userID,
	).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("load next user profile fact sequence for %q: %w", userID, err)
	}
	return sequence, nil
}

func insertUserProfileFact(
	ctx context.Context,
	db factDBTX,
	userID string,
	sequence int64,
	profile profileSnapshot,
	observedAt time.Time,
	source string,
	sourceID string,
) error {
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO user_profile_facts (
		     id,
		     user_id,
		     sequence,
		     login,
		     display_name,
		     avatar_url,
		     email,
		     observed_at_unix_nano,
		     source,
		     source_id,
		     inferred
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		newID("uprof"),
		userID,
		sequence,
		profile.Login,
		profile.DisplayName,
		profile.AvatarURL,
		profile.Email,
		observedAt.UTC().UnixNano(),
		source,
		sourceID,
		false,
	); err != nil {
		return fmt.Errorf("insert user profile fact for %q: %w", userID, err)
	}
	return nil
}
