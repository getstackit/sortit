package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sortit/internal/issues/issuesdb"
)

type enrichmentProjectionState struct {
	IssueID        string
	Status         IssueEnrichmentStatus
	Error          string
	TargetSequence int
	AttemptCount   int
	LeaseExpiresAt *time.Time
	LatestEventID  string
}

func updateIssueEnrichmentState(ctx context.Context, db issuesdb.DBTX, issueID string, fields IssueFieldUpdate) error {
	if fields.EnrichmentStatus == nil && fields.EnrichmentError == nil && fields.EnrichmentTargetSequence == nil {
		return nil
	}

	state, err := loadEnrichmentProjectionState(ctx, db, issueID)
	if err != nil {
		return err
	}

	if fields.EnrichmentStatus != nil {
		state.Status = normalizeIssueEnrichmentStatus(*fields.EnrichmentStatus)
	}
	if fields.EnrichmentError != nil {
		state.Error = strings.TrimSpace(*fields.EnrichmentError)
	}
	if fields.EnrichmentTargetSequence != nil {
		state.TargetSequence = *fields.EnrichmentTargetSequence
	}
	if state.TargetSequence <= 0 {
		state.TargetSequence = 1
	}
	if state.Status == EnrichmentStatusComplete {
		state.Error = ""
	}

	payload := mustEnrichmentJSON(map[string]any{
		"status":         string(state.Status),
		"error":          state.Error,
		"targetSequence": state.TargetSequence,
	})
	now := time.Now().UTC()
	eventID := NewEnrichmentEventID()
	if err := insertEnrichmentEvent(ctx, db, enrichmentEventRecord{
		ID:             eventID,
		IssueID:        issueID,
		TargetSequence: state.TargetSequence,
		Kind:           "state_updated",
		CreatedBy:      "system",
		CreatedAt:      now,
		Payload:        payload,
	}); err != nil {
		return err
	}

	state.LatestEventID = eventID
	return upsertEnrichmentProjectionState(ctx, db, state, now)
}

func syncEnrichmentProjectionFromQueue(
	ctx context.Context,
	db issuesdb.DBTX,
	issueID string,
	kind string,
) error {
	state, err := loadEnrichmentProjectionState(ctx, db, issueID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	eventID := NewEnrichmentEventID()
	payload := mustEnrichmentJSON(map[string]any{
		"status":                 string(state.Status),
		"error":                  state.Error,
		"targetSequence":         state.TargetSequence,
		"attemptCount":           state.AttemptCount,
		"leaseExpiresAtUnixNano": enrichmentLeaseUnixNano(state.LeaseExpiresAt),
	})
	if err := insertEnrichmentEvent(ctx, db, enrichmentEventRecord{
		ID:             eventID,
		IssueID:        issueID,
		TargetSequence: state.TargetSequence,
		Kind:           strings.TrimSpace(kind),
		CreatedBy:      "system",
		CreatedAt:      now,
		Payload:        payload,
	}); err != nil {
		return err
	}

	state.LatestEventID = eventID
	return upsertEnrichmentProjectionState(ctx, db, state, now)
}

type enrichmentEventRecord struct {
	ID             string
	IssueID        string
	TargetSequence int
	Kind           string
	CreatedBy      string
	CreatedAt      time.Time
	Payload        json.RawMessage
}

func loadEnrichmentProjectionState(
	ctx context.Context,
	db issuesdb.DBTX,
	issueID string,
) (enrichmentProjectionState, error) {
	var (
		state                  enrichmentProjectionState
		status                 string
		leaseExpiresAtUnixNano int64
	)
	if err := db.QueryRowContext(
		ctx,
		`SELECT i.id,
		        COALESCE(p.status, i.enrichment_status) AS status,
		        COALESCE(p.error, i.enrichment_error) AS error,
		        COALESCE(p.target_sequence, i.enrichment_target_sequence) AS target_sequence,
		        COALESCE(j.attempt_count, p.attempt_count, 0) AS attempt_count,
		        COALESCE(j.lease_expires_at_unix_nano, p.lease_expires_at_unix_nano, 0) AS lease_expires_at_unix_nano,
		        COALESCE(p.latest_event_id, '') AS latest_event_id
		 FROM issues i
		 LEFT JOIN issue_enrichment_projections p ON p.issue_id = i.id
		 LEFT JOIN issue_enrichment_jobs j ON j.issue_id = i.id
		 WHERE i.id = $1`,
		strings.TrimSpace(issueID),
	).Scan(
		&state.IssueID,
		&status,
		&state.Error,
		&state.TargetSequence,
		&state.AttemptCount,
		&leaseExpiresAtUnixNano,
		&state.LatestEventID,
	); err != nil {
		return enrichmentProjectionState{}, fmt.Errorf("load enrichment projection state for %q: %w", issueID, err)
	}

	state.Status = normalizeIssueEnrichmentStatus(IssueEnrichmentStatus(status))
	if state.TargetSequence <= 0 {
		state.TargetSequence = 1
	}
	if state.Status == EnrichmentStatusComplete {
		state.Error = ""
	}
	if leaseExpiresAtUnixNano > 0 {
		leaseExpiresAt := time.Unix(0, leaseExpiresAtUnixNano).UTC()
		state.LeaseExpiresAt = &leaseExpiresAt
	}
	return state, nil
}

func insertEnrichmentEvent(ctx context.Context, db issuesdb.DBTX, event enrichmentEventRecord) error {
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO issue_enrichment_events (
		     id,
		     issue_id,
		     target_sequence,
		     kind,
		     created_by,
		     created_at_unix_nano,
		     payload_json
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
		strings.TrimSpace(event.ID),
		strings.TrimSpace(event.IssueID),
		event.TargetSequence,
		strings.TrimSpace(event.Kind),
		strings.TrimSpace(event.CreatedBy),
		event.CreatedAt.UTC().UnixNano(),
		[]byte(event.Payload),
	); err != nil {
		return fmt.Errorf("insert enrichment event %q: %w", event.ID, err)
	}
	return nil
}

func upsertEnrichmentProjectionState(
	ctx context.Context,
	db issuesdb.DBTX,
	state enrichmentProjectionState,
	updatedAt time.Time,
) error {
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO issue_enrichment_projections (
		     issue_id,
		     status,
		     error,
		     target_sequence,
		     attempt_count,
		     lease_expires_at_unix_nano,
		     latest_event_id,
		     updated_at_unix_nano
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (issue_id) DO UPDATE
		 SET status = EXCLUDED.status,
		     error = EXCLUDED.error,
		     target_sequence = EXCLUDED.target_sequence,
		     attempt_count = EXCLUDED.attempt_count,
		     lease_expires_at_unix_nano = EXCLUDED.lease_expires_at_unix_nano,
		     latest_event_id = EXCLUDED.latest_event_id,
		     updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		strings.TrimSpace(state.IssueID),
		string(normalizeIssueEnrichmentStatus(state.Status)),
		strings.TrimSpace(state.Error),
		state.TargetSequence,
		state.AttemptCount,
		enrichmentLeaseUnixNano(state.LeaseExpiresAt),
		strings.TrimSpace(state.LatestEventID),
		updatedAt.UTC().UnixNano(),
	); err != nil {
		return fmt.Errorf("upsert enrichment projection for %q: %w", state.IssueID, err)
	}
	return nil
}

func enrichmentLeaseUnixNano(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.UTC().UnixNano()
}

func mustEnrichmentJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal enrichment payload: %v", err))
	}
	return json.RawMessage(encoded)
}
