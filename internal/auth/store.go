package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrUnauthorized = errors.New("unauthorized")
var ErrTokenNotFound = errors.New("token not found")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) UpsertOAuthUser(ctx context.Context, oauthUser OAuthUser) (User, error) {
	now := time.Now().UTC()
	incoming := profileSnapshot{
		Login:       strings.TrimSpace(oauthUser.Login),
		DisplayName: displayName(oauthUser.DisplayName, oauthUser.Login),
		AvatarURL:   strings.TrimSpace(oauthUser.AvatarURL),
		Email:       strings.TrimSpace(oauthUser.Email),
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin auth upsert tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var userID string
	err = tx.QueryRowContext(
		ctx,
		`SELECT user_id FROM auth_accounts WHERE provider = $1 AND provider_user_id = $2`,
		oauthUser.Provider,
		oauthUser.ProviderUserID,
	).Scan(&userID)
	switch {
	case err == nil:
		// Existing user: write the latest-profile projection and append a
		// profile-change fact only when the observed profile actually changed,
		// so routine logins don't churn the projection or the fact log.
		current, err := loadCurrentProfile(ctx, tx, userID)
		if err != nil {
			return User{}, err
		}
		if !current.equal(incoming) {
			if err := s.applyProfileChange(ctx, tx, userID, incoming, now); err != nil {
				return User{}, err
			}
		}
	case errors.Is(err, sql.ErrNoRows):
		userID = newID("user")
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO users (id, login, display_name, avatar_url, email, created_at_unix_nano, updated_at_unix_nano)
			 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
			userID,
			incoming.Login,
			incoming.DisplayName,
			incoming.AvatarURL,
			incoming.Email,
			now.UnixNano(),
		); err != nil {
			return User{}, fmt.Errorf("insert user: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO auth_accounts (id, user_id, provider, provider_user_id, created_at_unix_nano)
			 VALUES ($1, $2, $3, $4, $5)`,
			newID("acct"),
			userID,
			oauthUser.Provider,
			oauthUser.ProviderUserID,
			now.UnixNano(),
		); err != nil {
			return User{}, fmt.Errorf("insert auth account: %w", err)
		}
		// Seed the initial profile fact (sequence 1) alongside the projection.
		if err := insertUserProfileFact(ctx, tx, userID, 1, incoming, now, "user_create", userID+":created"); err != nil {
			return User{}, err
		}
	default:
		return User{}, fmt.Errorf("lookup auth account: %w", err)
	}

	user, err := selectUser(ctx, tx, userID)
	if err != nil {
		return User{}, err
	}

	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("commit auth upsert tx: %w", err)
	}

	return user, nil
}

// applyProfileChange writes the latest-profile projection and appends a
// matching append-only profile-change fact in the same transaction.
func (s *Store) applyProfileChange(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
	profile profileSnapshot,
	observedAt time.Time,
) error {
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE users
		 SET login = $1, display_name = $2, avatar_url = $3, email = $4, updated_at_unix_nano = $5
		 WHERE id = $6`,
		profile.Login,
		profile.DisplayName,
		profile.AvatarURL,
		profile.Email,
		observedAt.UTC().UnixNano(),
		userID,
	); err != nil {
		return fmt.Errorf("update user profile: %w", err)
	}

	sequence, err := nextUserProfileFactSequence(ctx, tx, userID)
	if err != nil {
		return err
	}
	return insertUserProfileFact(
		ctx,
		tx,
		userID,
		sequence,
		profile,
		observedAt,
		"user_login",
		fmt.Sprintf("%s:%d", userID, sequence),
	)
}

func (s *Store) CreateSession(ctx context.Context, userID string, expiresAt time.Time) (string, error) {
	rawToken, tokenHash, err := newSecretToken("sortitsession")
	if err != nil {
		return "", err
	}

	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO sessions (id, user_id, token_hash, expires_at_unix_nano, created_at_unix_nano)
		 VALUES ($1, $2, $3, $4, $5)`,
		newID("sess"),
		userID,
		tokenHash,
		expiresAt.UTC().UnixNano(),
		time.Now().UTC().UnixNano(),
	); err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}

	return rawToken, nil
}

func (s *Store) DeleteSession(ctx context.Context, rawToken string) error {
	tokenHash := hashToken(rawToken)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Store) LookupSession(ctx context.Context, rawToken string) (Principal, error) {
	return s.lookupPrincipal(
		ctx,
		`SELECT u.id, u.login, u.display_name, u.avatar_url, u.email
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at_unix_nano > $2`,
		hashToken(rawToken),
		time.Now().UTC().UnixNano(),
	)
}

func (s *Store) CreateAPIToken(ctx context.Context, userID, name string) (APIToken, string, error) {
	rawToken, tokenHash, err := newSecretToken("sortit")
	if err != nil {
		return APIToken{}, "", err
	}

	createdAt := time.Now().UTC()
	token := APIToken{
		ID:          newID("tok"),
		Name:        name,
		TokenPrefix: tokenPrefix(rawToken),
		CreatedAt:   createdAt,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return APIToken{}, "", fmt.Errorf("begin create api token tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Append-only lifecycle fact: a token is created exactly once (sequence 1).
	if err := insertAPITokenFact(ctx, tx, apiTokenFactRecord{
		ID:        newID("tokfact"),
		TokenID:   token.ID,
		UserID:    userID,
		Sequence:  1,
		Kind:      apiTokenFactKindCreated,
		CreatedBy: userID,
		CreatedAt: createdAt,
		Payload: mustAPITokenFactJSON(map[string]any{
			"name":              name,
			"tokenPrefix":       token.TokenPrefix,
			"createdAtUnixNano": createdAt.UnixNano(),
		}),
		Source:   "api_token_create",
		SourceID: token.ID + ":created",
	}); err != nil {
		return APIToken{}, "", err
	}

	// Current-state projection (hot-path lookup target).
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO api_tokens (id, user_id, token_hash, token_prefix, name, created_at_unix_nano, revoked_at_unix_nano)
		 VALUES ($1, $2, $3, $4, $5, $6, 0)`,
		token.ID,
		userID,
		tokenHash,
		token.TokenPrefix,
		name,
		createdAt.UnixNano(),
	); err != nil {
		return APIToken{}, "", fmt.Errorf("insert api token: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return APIToken{}, "", fmt.Errorf("commit create api token tx: %w", err)
	}

	return token, rawToken, nil
}

func (s *Store) ListAPITokens(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, name, token_prefix, created_at_unix_nano, revoked_at_unix_nano, last_used_at_unix_nano
		 FROM api_tokens
		 WHERE user_id = $1
		 ORDER BY created_at_unix_nano DESC, id ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var tokens []APIToken
	for rows.Next() {
		var token APIToken
		var createdAt int64
		var revokedAt int64
		var lastUsedAt int64
		if err := rows.Scan(&token.ID, &token.Name, &token.TokenPrefix, &createdAt, &revokedAt, &lastUsedAt); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		token.CreatedAt = time.Unix(0, createdAt).UTC()
		if revokedAt > 0 {
			revokedAtTime := time.Unix(0, revokedAt).UTC()
			token.RevokedAt = &revokedAtTime
		}
		if lastUsedAt > 0 {
			lastUsedAtTime := time.Unix(0, lastUsedAt).UTC()
			token.LastUsedAt = &lastUsedAtTime
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api tokens: %w", err)
	}
	return tokens, nil
}

func (s *Store) RevokeAPIToken(ctx context.Context, userID, tokenID string) error {
	tokenID = strings.TrimSpace(tokenID)
	revokedAt := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin revoke api token tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Update the current-state projection first; the WHERE clause enforces
	// ownership and the not-already-revoked guard. Only append a 'revoked' fact
	// when a row actually changed, so re-revoking is a no-op rather than a
	// spurious audit entry.
	result, err := tx.ExecContext(
		ctx,
		`UPDATE api_tokens
		 SET revoked_at_unix_nano = $1
		 WHERE id = $2 AND user_id = $3 AND revoked_at_unix_nano = 0`,
		revokedAt.UnixNano(),
		tokenID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke api token rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrTokenNotFound
	}

	sequence, err := nextAPITokenFactSequence(ctx, tx, tokenID)
	if err != nil {
		return err
	}
	if err := insertAPITokenFact(ctx, tx, apiTokenFactRecord{
		ID:        newID("tokfact"),
		TokenID:   tokenID,
		UserID:    userID,
		Sequence:  sequence,
		Kind:      apiTokenFactKindRevoked,
		CreatedBy: userID,
		CreatedAt: revokedAt,
		Payload: mustAPITokenFactJSON(map[string]any{
			"revokedAtUnixNano": revokedAt.UnixNano(),
		}),
		Source:   "api_token_revoke",
		SourceID: tokenID + ":revoked",
	}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit revoke api token tx: %w", err)
	}
	return nil
}

func (s *Store) LookupAPIToken(ctx context.Context, rawToken string) (Principal, error) {
	tokenHash := hashToken(rawToken)
	principal, err := s.lookupPrincipal(
		ctx,
		`SELECT u.id, u.login, u.display_name, u.avatar_url, u.email
		 FROM api_tokens t
		 JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = $1 AND t.revoked_at_unix_nano = 0`,
		tokenHash,
	)
	if err != nil {
		return Principal{}, err
	}

	// Best-effort update of last-used timestamp.
	_, _ = s.db.ExecContext(
		ctx,
		`UPDATE api_tokens SET last_used_at_unix_nano = $1 WHERE token_hash = $2`,
		time.Now().UTC().UnixNano(),
		tokenHash,
	)

	return principal, nil
}

func (s *Store) lookupPrincipal(ctx context.Context, query string, args ...any) (Principal, error) {
	var principal Principal
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&principal.UserID,
		&principal.Login,
		&principal.DisplayName,
		&principal.AvatarURL,
		&principal.Email,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrUnauthorized
	}
	if err != nil {
		return Principal{}, fmt.Errorf("lookup principal: %w", err)
	}
	return principal, nil
}

func selectUser(ctx context.Context, querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, userID string) (User, error) {
	var user User
	var createdAt int64
	var updatedAt int64
	err := querier.QueryRowContext(
		ctx,
		`SELECT id, login, display_name, avatar_url, email, created_at_unix_nano, updated_at_unix_nano
		 FROM users
		 WHERE id = $1`,
		userID,
	).Scan(
		&user.ID,
		&user.Login,
		&user.DisplayName,
		&user.AvatarURL,
		&user.Email,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return User{}, fmt.Errorf("select user: %w", err)
	}
	user.CreatedAt = time.Unix(0, createdAt).UTC()
	user.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return user, nil
}

func displayName(displayNameValue, login string) string {
	displayNameValue = strings.TrimSpace(displayNameValue)
	if displayNameValue != "" {
		return displayNameValue
	}
	return strings.TrimSpace(login)
}

func newSecretToken(prefix string) (string, string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", "", fmt.Errorf("generate random token: %w", err)
	}
	raw := prefix + "_" + base64.RawURLEncoding.EncodeToString(buffer)
	return raw, hashToken(raw), nil
}

func tokenPrefix(raw string) string {
	if len(raw) <= 14 {
		return raw
	}
	return raw[:14]
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newID(prefix string) string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(buffer)
}
