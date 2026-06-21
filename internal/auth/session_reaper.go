package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	defaultSessionReapInterval  = 15 * time.Minute
	defaultSessionReapBatchSize = 500
)

// SessionReapResult summarizes one ReapExpiredSessions call. Scanned is the number
// of expired projection rows examined (bounded by batchSize); Reaped is how many
// were actually invalidated (a candidate refreshed or logged out between the scan
// and the per-row tx is skipped, so Reaped <= Scanned).
type SessionReapResult struct {
	Scanned int
	Reaped  int
}

// ReapExpiredSessions invalidates sessions whose projection row has passed its
// expiry. For each expired session it appends an 'invalidated'/reason='expired'
// fact and deletes the projection row in one tx — mirroring DeleteSession's logout
// path, but for TTL expiry. It is idempotent and concurrency-safe: the fact is
// written only when the projection row is actually deleted, so a session removed by
// a concurrent logout (or an earlier sweep) yields no duplicate fact.
//
// The auth read path (LookupSession) already filters expires_at > now, so this is
// projection hygiene + audit-trail completeness, not a correctness gate: it bounds
// projection growth and gives expiry the same lifecycle fact that logout gets.
func (s *Store) ReapExpiredSessions(ctx context.Context, now time.Time, batchSize int) (SessionReapResult, error) {
	if batchSize <= 0 {
		batchSize = defaultSessionReapBatchSize
	}
	cutoff := now.UTC().UnixNano()

	ids, err := s.expiredSessionIDs(ctx, cutoff, batchSize)
	if err != nil {
		return SessionReapResult{}, err
	}

	result := SessionReapResult{Scanned: len(ids)}
	for _, id := range ids {
		reaped, err := s.reapExpiredSession(ctx, id, cutoff)
		if err != nil {
			return result, err
		}
		if reaped {
			result.Reaped++
		}
	}
	return result, nil
}

func (s *Store) expiredSessionIDs(ctx context.Context, cutoff int64, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id FROM sessions WHERE expires_at_unix_nano <= $1 ORDER BY id LIMIT $2`,
		cutoff,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list expired sessions: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan expired session id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired sessions: %w", err)
	}
	return ids, nil
}

// reapExpiredSession removes one expired projection row and appends the matching
// 'expired' fact in a single tx. The DELETE ... RETURNING re-checks expiry inside
// the tx, so a session refreshed or logged out between the candidate scan and here
// is left untouched (returns false, no fact written).
func (s *Store) reapExpiredSession(ctx context.Context, sessionID string, cutoff int64) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin reap session tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var userID string
	err = tx.QueryRowContext(
		ctx,
		`DELETE FROM sessions WHERE id = $1 AND expires_at_unix_nano <= $2 RETURNING user_id`,
		sessionID,
		cutoff,
	).Scan(&userID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("delete expired session %q: %w", sessionID, err)
	}

	sequence, err := nextSessionFactSequence(ctx, tx, sessionID)
	if err != nil {
		return false, err
	}
	if err := insertSessionFact(ctx, tx, sessionFactRecord{
		ID:        newID("sessfact"),
		SessionID: sessionID,
		UserID:    userID,
		Sequence:  sequence,
		Kind:      sessionFactKindInvalidated,
		Reason:    sessionReasonExpired,
		CreatedAt: time.Now().UTC(),
		Source:    "session_expire",
		SourceID:  sessionID + ":expired",
	}); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit reap session tx: %w", err)
	}
	return true, nil
}

// SessionReaper periodically invalidates expired sessions, keeping the auth read
// path read-only — LookupSession never writes — while still bounding the
// active-session projection and giving expiry a lifecycle fact.
type SessionReaper struct {
	Store     *Store
	Logger    *slog.Logger
	Interval  time.Duration
	BatchSize int
}

// Run sweeps on an interval until ctx is canceled. It sweeps once immediately on
// start so a server restart clears any backlog without waiting a full interval.
func (r *SessionReaper) Run(ctx context.Context) error {
	interval := r.Interval
	if interval <= 0 {
		interval = defaultSessionReapInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if _, err := r.sweep(ctx); err != nil && ctx.Err() == nil {
			r.logger().WarnContext(ctx, "session reaper sweep failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// sweep drains every currently expired session, looping in BatchSize chunks until a
// scan returns a short batch (no more expired rows). It returns the total reaped.
func (r *SessionReaper) sweep(ctx context.Context) (int, error) {
	if r == nil || r.Store == nil {
		return 0, nil
	}
	batchSize := r.BatchSize
	if batchSize <= 0 {
		batchSize = defaultSessionReapBatchSize
	}

	total := 0
	for {
		res, err := r.Store.ReapExpiredSessions(ctx, time.Now().UTC(), batchSize)
		if err != nil {
			return total, err
		}
		total += res.Reaped
		if res.Scanned < batchSize || ctx.Err() != nil {
			break
		}
	}
	if total > 0 {
		r.logger().InfoContext(ctx, "reaped expired sessions", "count", total)
	}
	return total, nil
}

func (r *SessionReaper) logger() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}
