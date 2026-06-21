package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// API token lifecycle/audit fact kinds written to the append-only api_token_facts
// log. The api_tokens table remains the current-state projection; only lifecycle
// events (create/revoke) become facts. last_used_at stays in-place telemetry.
const (
	apiTokenFactKindCreated = "created"
	apiTokenFactKindRevoked = "revoked"
)

// factDBTX is satisfied by both *sql.DB and *sql.Tx so fact helpers can run inside
// the create/revoke transactions that also write the api_tokens projection.
type factDBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type apiTokenFactRecord struct {
	ID        string
	TokenID   string
	UserID    string
	Sequence  int64
	Kind      string
	CreatedBy string
	CreatedAt time.Time
	Payload   json.RawMessage
	Source    string
	SourceID  string
}

func insertAPITokenFact(ctx context.Context, db factDBTX, fact apiTokenFactRecord) error {
	payload := fact.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO api_token_facts (
		     id,
		     token_id,
		     user_id,
		     sequence,
		     kind,
		     created_by,
		     created_at_unix_nano,
		     payload_json,
		     source,
		     source_id,
		     inferred
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11)`,
		strings.TrimSpace(fact.ID),
		strings.TrimSpace(fact.TokenID),
		strings.TrimSpace(fact.UserID),
		fact.Sequence,
		strings.TrimSpace(fact.Kind),
		strings.TrimSpace(fact.CreatedBy),
		fact.CreatedAt.UTC().UnixNano(),
		[]byte(payload),
		strings.TrimSpace(fact.Source),
		strings.TrimSpace(fact.SourceID),
		false,
	); err != nil {
		return fmt.Errorf("insert api token fact %q: %w", fact.ID, err)
	}
	return nil
}

func nextAPITokenFactSequence(ctx context.Context, db factDBTX, tokenID string) (int64, error) {
	var sequence int64
	if err := db.QueryRowContext(
		ctx,
		`SELECT COALESCE(MAX(sequence), 0) + 1 FROM api_token_facts WHERE token_id = $1`,
		strings.TrimSpace(tokenID),
	).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("load next api token fact sequence for %q: %w", tokenID, err)
	}
	return sequence, nil
}

func mustAPITokenFactJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal api token fact payload: %v", err))
	}
	return json.RawMessage(encoded)
}
