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

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) UpsertOAuthUser(ctx context.Context, oauthUser OAuthUser) (User, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin auth upsert tx: %w", err)
	}
	defer tx.Rollback()

	var userID string
	err = tx.QueryRowContext(
		ctx,
		`SELECT user_id FROM auth_accounts WHERE provider = ? AND provider_user_id = ?`,
		oauthUser.Provider,
		oauthUser.ProviderUserID,
	).Scan(&userID)
	switch {
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		userID = newID("user")
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO users (id, login, display_name, avatar_url, email, created_at_unix_nano, updated_at_unix_nano)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			userID,
			strings.TrimSpace(oauthUser.Login),
			displayName(oauthUser.DisplayName, oauthUser.Login),
			strings.TrimSpace(oauthUser.AvatarURL),
			strings.TrimSpace(oauthUser.Email),
			now.UnixNano(),
			now.UnixNano(),
		); err != nil {
			return User{}, fmt.Errorf("insert user: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO auth_accounts (id, user_id, provider, provider_user_id, created_at_unix_nano)
			 VALUES (?, ?, ?, ?, ?)`,
			newID("acct"),
			userID,
			oauthUser.Provider,
			oauthUser.ProviderUserID,
			now.UnixNano(),
		); err != nil {
			return User{}, fmt.Errorf("insert auth account: %w", err)
		}
	default:
		return User{}, fmt.Errorf("lookup auth account: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE users
		 SET login = ?, display_name = ?, avatar_url = ?, email = ?, updated_at_unix_nano = ?
		 WHERE id = ?`,
		strings.TrimSpace(oauthUser.Login),
		displayName(oauthUser.DisplayName, oauthUser.Login),
		strings.TrimSpace(oauthUser.AvatarURL),
		strings.TrimSpace(oauthUser.Email),
		now.UnixNano(),
		userID,
	); err != nil {
		return User{}, fmt.Errorf("update user profile: %w", err)
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

func (s *SQLiteStore) CreateSession(ctx context.Context, userID string, expiresAt time.Time) (string, error) {
	rawToken, tokenHash, err := newSecretToken("sst")
	if err != nil {
		return "", err
	}

	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO sessions (id, user_id, token_hash, expires_at_unix_nano, created_at_unix_nano)
		 VALUES (?, ?, ?, ?, ?)`,
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

func (s *SQLiteStore) DeleteSession(ctx context.Context, rawToken string) error {
	tokenHash := hashToken(rawToken)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LookupSession(ctx context.Context, rawToken string) (Principal, error) {
	return s.lookupPrincipal(
		ctx,
		`SELECT u.id, u.login, u.display_name, u.avatar_url, u.email
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = ? AND s.expires_at_unix_nano > ?`,
		hashToken(rawToken),
		time.Now().UTC().UnixNano(),
	)
}

func (s *SQLiteStore) CreateAPIToken(ctx context.Context, userID string) (APIToken, string, error) {
	rawToken, tokenHash, err := newSecretToken("spt")
	if err != nil {
		return APIToken{}, "", err
	}

	createdAt := time.Now().UTC()
	token := APIToken{
		ID:          newID("tok"),
		TokenPrefix: tokenPrefix(rawToken),
		CreatedAt:   createdAt,
	}

	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO api_tokens (id, user_id, token_hash, token_prefix, created_at_unix_nano, revoked_at_unix_nano)
		 VALUES (?, ?, ?, ?, ?, 0)`,
		token.ID,
		userID,
		tokenHash,
		token.TokenPrefix,
		createdAt.UnixNano(),
	); err != nil {
		return APIToken{}, "", fmt.Errorf("insert api token: %w", err)
	}

	return token, rawToken, nil
}

func (s *SQLiteStore) ListAPITokens(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, token_prefix, created_at_unix_nano, revoked_at_unix_nano
		 FROM api_tokens
		 WHERE user_id = ?
		 ORDER BY created_at_unix_nano DESC, id ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	var tokens []APIToken
	for rows.Next() {
		var token APIToken
		var createdAt int64
		var revokedAt int64
		if err := rows.Scan(&token.ID, &token.TokenPrefix, &createdAt, &revokedAt); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		token.CreatedAt = time.Unix(0, createdAt).UTC()
		if revokedAt > 0 {
			revokedAtTime := time.Unix(0, revokedAt).UTC()
			token.RevokedAt = &revokedAtTime
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api tokens: %w", err)
	}
	return tokens, nil
}

func (s *SQLiteStore) RevokeAPIToken(ctx context.Context, userID, tokenID string) error {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE api_tokens
		 SET revoked_at_unix_nano = ?
		 WHERE id = ? AND user_id = ? AND revoked_at_unix_nano = 0`,
		time.Now().UTC().UnixNano(),
		strings.TrimSpace(tokenID),
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
	return nil
}

func (s *SQLiteStore) LookupAPIToken(ctx context.Context, rawToken string) (Principal, error) {
	return s.lookupPrincipal(
		ctx,
		`SELECT u.id, u.login, u.display_name, u.avatar_url, u.email
		 FROM api_tokens t
		 JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = ? AND t.revoked_at_unix_nano = 0`,
		hashToken(rawToken),
	)
}

func (s *SQLiteStore) lookupPrincipal(ctx context.Context, query string, args ...any) (Principal, error) {
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
		 WHERE id = ?`,
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
