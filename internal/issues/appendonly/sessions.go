package appendonly

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	defaultSessionCheckpoint = "session-backfill"
	sessionDomain            = "sessions"

	// sessionKindInvalidated mirrors the runtime fact kind written by
	// internal/auth on logout. The 'created' kind reuses the package-level
	// kindCreated constant.
	sessionKindInvalidated = "invalidated"
)

// SessionBackfillResult summarizes one batch of the session_facts backfill.
type SessionBackfillResult struct {
	Checkpoint        string `json:"checkpoint"`
	ProcessedSessions int    `json:"processedSessions"`
	WroteFacts        int    `json:"wroteFacts"`
	LastSessionID     string `json:"lastSessionId,omitempty"`
	Complete          bool   `json:"complete"`
}

// SessionParityMismatch records a single active session whose session_facts
// history does not agree with the sessions projection.
type SessionParityMismatch struct {
	SessionID string   `json:"sessionId"`
	Fields    []string `json:"fields"`
}

// SessionParityResult reports how well the append-only facts reproduce the
// active-session projection.
type SessionParityResult struct {
	Domain             string                  `json:"domain"`
	TotalSessions      int                     `json:"totalSessions"`
	SessionsWithFacts  int                     `json:"sessionsWithFacts"`
	MissingFacts       int                     `json:"missingFacts"`
	MismatchedSessions int                     `json:"mismatchedSessions"`
	Mismatches         []SessionParityMismatch `json:"mismatches,omitempty"`
}

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

type sessionLegacyRow struct {
	ID        string
	UserID    string
	CreatedAt int64
}

type sessionCursor struct {
	LastSessionID string `json:"lastSessionId"`
}

type sessionFactSummary struct {
	HasCreated     bool
	HasInvalidated bool
}

// BackfillBatch seeds session_facts from the active-session projection: one
// 'created' fact per existing session. (Past logouts were destructive deletes, so
// there is no history to reconstruct for them — only currently-active sessions
// remain.) It is idempotent and additive — a session that already has a fact is
// left untouched, avoiding collisions on the unique (session_id, sequence) index.
func (s *SessionStore) BackfillBatch(
	ctx context.Context,
	checkpointName string,
	batchSize int,
) (SessionBackfillResult, error) {
	checkpointName = strings.TrimSpace(checkpointName)
	if checkpointName == "" {
		checkpointName = defaultSessionCheckpoint
	}
	if batchSize <= 0 {
		batchSize = 100
	}

	framework := NewFrameworkStore(s.db)
	checkpoint, ok, err := framework.GetCheckpoint(ctx, checkpointName)
	if err != nil {
		return SessionBackfillResult{}, err
	}

	cursor := sessionCursor{}
	if ok && len(checkpoint.CursorJSON) > 0 {
		if err := json.Unmarshal(checkpoint.CursorJSON, &cursor); err != nil {
			return SessionBackfillResult{}, fmt.Errorf("decode session checkpoint cursor: %w", err)
		}
	}

	sessions, err := s.listSessions(ctx)
	if err != nil {
		return SessionBackfillResult{}, err
	}

	candidates := make([]sessionLegacyRow, 0, batchSize)
	remainingCount := 0
	for _, session := range sessions {
		if cursor.LastSessionID != "" && session.ID <= cursor.LastSessionID {
			continue
		}
		remainingCount++
		if len(candidates) < batchSize {
			candidates = append(candidates, session)
		}
	}

	result := SessionBackfillResult{
		Checkpoint: checkpointName,
		Complete:   len(candidates) == 0,
	}
	if len(candidates) == 0 {
		if err := framework.UpsertCheckpoint(ctx, Checkpoint{
			Name:        checkpointName,
			Phase:       phaseCompleted,
			CursorJSON:  mustMarshalJSON(cursor),
			SummaryJSON: mustMarshalJSON(result),
		}); err != nil {
			return SessionBackfillResult{}, err
		}
		return result, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionBackfillResult{}, fmt.Errorf("begin session backfill tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, session := range candidates {
		summary, err := loadSessionFactSummary(ctx, tx, session.ID)
		if err != nil {
			return SessionBackfillResult{}, err
		}
		if !summary.HasCreated {
			if err := insertBackfillSessionFact(ctx, tx, session); err != nil {
				return SessionBackfillResult{}, err
			}
			result.WroteFacts++
		}
		result.ProcessedSessions++
		result.LastSessionID = session.ID
	}

	if err := tx.Commit(); err != nil {
		return SessionBackfillResult{}, fmt.Errorf("commit session backfill tx: %w", err)
	}

	result.Complete = remainingCount <= batchSize
	cursor.LastSessionID = result.LastSessionID
	phase := phaseBackfilling
	if result.Complete {
		phase = phaseCompleted
	}
	if err := framework.UpsertCheckpoint(ctx, Checkpoint{
		Name:        checkpointName,
		Phase:       phase,
		CursorJSON:  mustMarshalJSON(cursor),
		SummaryJSON: mustMarshalJSON(result),
	}); err != nil {
		return SessionBackfillResult{}, err
	}

	return result, nil
}

// CheckParity verifies that every active session has a 'created' fact and is not
// contradicted by an 'invalidated' fact (an invalidated session should have been
// removed from the projection).
func (s *SessionStore) CheckParity(ctx context.Context, detailLimit int) (SessionParityResult, error) {
	if detailLimit <= 0 {
		detailLimit = 20
	}

	sessions, err := s.listSessions(ctx)
	if err != nil {
		return SessionParityResult{}, err
	}
	summaries, err := s.loadAllFactSummaries(ctx)
	if err != nil {
		return SessionParityResult{}, err
	}

	result := SessionParityResult{
		Domain:            sessionDomain,
		TotalSessions:     len(sessions),
		SessionsWithFacts: len(summaries),
	}

	for _, session := range sessions {
		summary := summaries[session.ID]
		fields := make([]string, 0, 2)
		if !summary.HasCreated {
			fields = append(fields, "missing_created_fact")
		}
		if summary.HasInvalidated {
			fields = append(fields, "invalidated_but_active")
		}
		if len(fields) == 0 {
			continue
		}
		if !summary.HasCreated {
			result.MissingFacts++
		}
		result.MismatchedSessions++
		if len(result.Mismatches) < detailLimit {
			result.Mismatches = append(result.Mismatches, SessionParityMismatch{
				SessionID: session.ID,
				Fields:    fields,
			})
		}
	}

	return result, nil
}

func insertBackfillSessionFact(ctx context.Context, tx *sql.Tx, session sessionLegacyRow) error {
	if _, err := tx.ExecContext(
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
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (source, source_id) DO NOTHING`,
		"backfill-session-created:"+session.ID,
		session.ID,
		session.UserID,
		1,
		kindCreated,
		"",
		session.CreatedAt,
		"legacy_session",
		session.ID+":created",
		true,
	); err != nil {
		return fmt.Errorf("insert backfill session created fact for %q: %w", session.ID, err)
	}
	return nil
}

func (s *SessionStore) listSessions(ctx context.Context) ([]sessionLegacyRow, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, user_id, created_at_unix_nano FROM sessions`,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions for backfill: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var sessions []sessionLegacyRow
	for rows.Next() {
		var session sessionLegacyRow
		if err := rows.Scan(&session.ID, &session.UserID, &session.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session rows: %w", err)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ID < sessions[j].ID
	})
	return sessions, nil
}

func (s *SessionStore) loadAllFactSummaries(ctx context.Context) (map[string]sessionFactSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT session_id, kind FROM session_facts`)
	if err != nil {
		return nil, fmt.Errorf("list session facts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	summaries := make(map[string]sessionFactSummary)
	for rows.Next() {
		var sessionID, kind string
		if err := rows.Scan(&sessionID, &kind); err != nil {
			return nil, fmt.Errorf("scan session fact: %w", err)
		}
		summary := summaries[sessionID]
		applySessionFactKind(&summary, kind)
		summaries[sessionID] = summary
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session facts: %w", err)
	}
	return summaries, nil
}

func loadSessionFactSummary(ctx context.Context, tx *sql.Tx, sessionID string) (sessionFactSummary, error) {
	rows, err := tx.QueryContext(ctx, `SELECT kind FROM session_facts WHERE session_id = $1`, sessionID)
	if err != nil {
		return sessionFactSummary{}, fmt.Errorf("load session fact kinds for %q: %w", sessionID, err)
	}
	defer rows.Close() //nolint:errcheck

	var summary sessionFactSummary
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			return sessionFactSummary{}, fmt.Errorf("scan session fact kind for %q: %w", sessionID, err)
		}
		applySessionFactKind(&summary, kind)
	}
	if err := rows.Err(); err != nil {
		return sessionFactSummary{}, fmt.Errorf("iterate session fact kinds for %q: %w", sessionID, err)
	}
	return summary, nil
}

func applySessionFactKind(summary *sessionFactSummary, kind string) {
	switch strings.TrimSpace(kind) {
	case kindCreated:
		summary.HasCreated = true
	case sessionKindInvalidated:
		summary.HasInvalidated = true
	}
}
