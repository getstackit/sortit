package auth

import (
	"context"
	"fmt"
	"time"
)

// Session lifecycle fact kinds / reasons written to the append-only session_facts
// log. sessions remains the active-session projection; only lifecycle events
// (created/invalidated) become facts.
const (
	sessionFactKindCreated     = "created"
	sessionFactKindInvalidated = "invalidated"
	sessionReasonLogout        = "logout"
)

type sessionFactRecord struct {
	ID        string
	SessionID string
	UserID    string
	Sequence  int64
	Kind      string
	Reason    string
	CreatedAt time.Time
	Source    string
	SourceID  string
}

func insertSessionFact(ctx context.Context, db factDBTX, fact sessionFactRecord) error {
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO session_facts (
		     id,
		     session_id,
		     user_id,
		     sequence,
		     kind,
		     reason,
		     created_at_unix_nano,
		     source,
		     source_id,
		     inferred
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		fact.ID,
		fact.SessionID,
		fact.UserID,
		fact.Sequence,
		fact.Kind,
		fact.Reason,
		fact.CreatedAt.UTC().UnixNano(),
		fact.Source,
		fact.SourceID,
		false,
	); err != nil {
		return fmt.Errorf("insert session fact %q: %w", fact.ID, err)
	}
	return nil
}

func nextSessionFactSequence(ctx context.Context, db factDBTX, sessionID string) (int64, error) {
	var sequence int64
	if err := db.QueryRowContext(
		ctx,
		`SELECT COALESCE(MAX(sequence), 0) + 1 FROM session_facts WHERE session_id = $1`,
		sessionID,
	).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("load next session fact sequence for %q: %w", sessionID, err)
	}
	return sequence, nil
}
