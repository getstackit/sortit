package appendonly

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"splat/internal/issues"
)

const (
	defaultEnrichmentCheckpoint = "issue-enrichment-backfill"
	enrichmentDomain            = "issue-enrichment"
)

type EnrichmentProjection struct {
	IssueID        string                       `json:"issueId"`
	Status         issues.IssueEnrichmentStatus `json:"status"`
	Error          string                       `json:"error,omitempty"`
	TargetSequence int                          `json:"targetSequence"`
	AttemptCount   int                          `json:"attemptCount"`
	LeaseExpiresAt *time.Time                   `json:"leaseExpiresAt,omitempty"`
	LatestEventID  string                       `json:"latestEventId,omitempty"`
	UpdatedAt      time.Time                    `json:"updatedAt"`
}

type EnrichmentBackfillResult struct {
	Checkpoint       string `json:"checkpoint"`
	ProcessedIssues  int    `json:"processedIssues"`
	WroteEvents      int    `json:"wroteEvents"`
	WroteProjections int    `json:"wroteProjections"`
	LastIssueID      string `json:"lastIssueId,omitempty"`
	Complete         bool   `json:"complete"`
}

type EnrichmentParityMismatch struct {
	IssueID string   `json:"issueId"`
	Fields  []string `json:"fields"`
}

type EnrichmentParityResult struct {
	Domain             string                     `json:"domain"`
	TotalIssues        int                        `json:"totalIssues"`
	ProjectionCount    int                        `json:"projectionCount"`
	MissingProjections int                        `json:"missingProjections"`
	MismatchedIssues   int                        `json:"mismatchedIssues"`
	Mismatches         []EnrichmentParityMismatch `json:"mismatches,omitempty"`
}

type enrichmentStateRow struct {
	IssueID        string
	Status         issues.IssueEnrichmentStatus
	Error          string
	TargetSequence int
	AttemptCount   int
	LeaseExpiresAt *time.Time
	CreatedAt      time.Time
	ObservedAt     time.Time
}

type EnrichmentStore struct {
	db *sql.DB
}

func NewEnrichmentStore(db *sql.DB) *EnrichmentStore {
	return &EnrichmentStore{db: db}
}

func (s *EnrichmentStore) BackfillBatch(
	ctx context.Context,
	checkpointName string,
	batchSize int,
) (EnrichmentBackfillResult, error) {
	checkpointName = strings.TrimSpace(checkpointName)
	if checkpointName == "" {
		checkpointName = defaultEnrichmentCheckpoint
	}
	if batchSize <= 0 {
		batchSize = 100
	}

	framework := NewFrameworkStore(s.db)
	checkpoint, ok, err := framework.GetCheckpoint(ctx, checkpointName)
	if err != nil {
		return EnrichmentBackfillResult{}, err
	}

	cursor := lifecycleCursor{}
	if ok && len(checkpoint.CursorJSON) > 0 {
		if err := json.Unmarshal(checkpoint.CursorJSON, &cursor); err != nil {
			return EnrichmentBackfillResult{}, fmt.Errorf("decode enrichment checkpoint cursor: %w", err)
		}
	}

	rows, err := s.listEnrichmentStateRows(ctx)
	if err != nil {
		return EnrichmentBackfillResult{}, err
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].IssueID < rows[j].IssueID
	})

	remainingCount := 0
	candidates := make([]enrichmentStateRow, 0, batchSize)
	for _, row := range rows {
		if cursor.LastIssueID != "" && row.IssueID <= cursor.LastIssueID {
			continue
		}
		remainingCount++
		if len(candidates) < batchSize {
			candidates = append(candidates, row)
		}
	}

	result := EnrichmentBackfillResult{
		Checkpoint: checkpointName,
		Complete:   len(candidates) == 0,
	}
	if len(candidates) == 0 {
		if err := framework.UpsertCheckpoint(ctx, Checkpoint{
			Name:        checkpointName,
			Phase:       "completed",
			CursorJSON:  mustMarshalJSON(cursor),
			SummaryJSON: mustMarshalJSON(result),
		}); err != nil {
			return EnrichmentBackfillResult{}, err
		}
		return result, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EnrichmentBackfillResult{}, fmt.Errorf("begin enrichment backfill tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, row := range candidates {
		event, projection := deriveEnrichmentBackfill(row)
		if err := replaceEnrichmentEvents(ctx, tx, row, event); err != nil {
			return EnrichmentBackfillResult{}, err
		}
		if err := upsertEnrichmentProjection(ctx, tx, projection); err != nil {
			return EnrichmentBackfillResult{}, err
		}

		result.ProcessedIssues++
		result.WroteEvents++
		result.WroteProjections++
		result.LastIssueID = row.IssueID
	}

	if err := tx.Commit(); err != nil {
		return EnrichmentBackfillResult{}, fmt.Errorf("commit enrichment backfill tx: %w", err)
	}

	result.Complete = remainingCount <= batchSize
	cursor.LastIssueID = result.LastIssueID
	phase := "backfilling"
	if result.Complete {
		phase = "completed"
	}
	if err := framework.UpsertCheckpoint(ctx, Checkpoint{
		Name:        checkpointName,
		Phase:       phase,
		CursorJSON:  mustMarshalJSON(cursor),
		SummaryJSON: mustMarshalJSON(result),
	}); err != nil {
		return EnrichmentBackfillResult{}, err
	}

	return result, nil
}

func (s *EnrichmentStore) CheckParity(
	ctx context.Context,
	detailLimit int,
) (EnrichmentParityResult, error) {
	if detailLimit <= 0 {
		detailLimit = 20
	}

	rows, err := s.listEnrichmentStateRows(ctx)
	if err != nil {
		return EnrichmentParityResult{}, err
	}
	projections, err := s.loadEnrichmentProjections(ctx)
	if err != nil {
		return EnrichmentParityResult{}, err
	}

	result := EnrichmentParityResult{
		Domain:          enrichmentDomain,
		TotalIssues:     len(rows),
		ProjectionCount: len(projections),
	}

	for _, row := range rows {
		projection, ok := projections[row.IssueID]
		if !ok {
			result.MissingProjections++
			result.MismatchedIssues++
			if len(result.Mismatches) < detailLimit {
				result.Mismatches = append(result.Mismatches, EnrichmentParityMismatch{
					IssueID: row.IssueID,
					Fields:  []string{"missing_projection"},
				})
			}
			continue
		}

		fields := compareEnrichmentProjection(row, projection)
		if len(fields) == 0 {
			continue
		}
		result.MismatchedIssues++
		if len(result.Mismatches) < detailLimit {
			result.Mismatches = append(result.Mismatches, EnrichmentParityMismatch{
				IssueID: row.IssueID,
				Fields:  fields,
			})
		}
	}

	return result, nil
}

func (s *EnrichmentStore) listEnrichmentStateRows(ctx context.Context) ([]enrichmentStateRow, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT i.id,
		        COALESCE(p.status, i.enrichment_status) AS enrichment_status,
		        COALESCE(p.error, i.enrichment_error) AS enrichment_error,
		        COALESCE(p.target_sequence, i.enrichment_target_sequence) AS enrichment_target_sequence,
		        COALESCE(j.attempt_count, 0) AS attempt_count,
		        COALESCE(j.lease_expires_at_unix_nano, 0) AS lease_expires_at_unix_nano,
		        i.created_at_unix_nano,
		        COALESCE(p.updated_at_unix_nano, j.updated_at_unix_nano, i.created_at_unix_nano) AS observed_at_unix_nano
		 FROM issues i
		 LEFT JOIN issue_enrichment_projections p ON p.issue_id = i.id
		 LEFT JOIN issue_enrichment_jobs j ON j.issue_id = i.id
		 ORDER BY i.id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list enrichment source rows: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	items := make([]enrichmentStateRow, 0)
	for rows.Next() {
		var (
			item                   enrichmentStateRow
			status                 string
			leaseExpiresAtUnixNano int64
			createdAtUnixNano      int64
			observedAtUnixNano     int64
		)
		if err := rows.Scan(
			&item.IssueID,
			&status,
			&item.Error,
			&item.TargetSequence,
			&item.AttemptCount,
			&leaseExpiresAtUnixNano,
			&createdAtUnixNano,
			&observedAtUnixNano,
		); err != nil {
			return nil, fmt.Errorf("scan enrichment source row: %w", err)
		}
		item.Status = issues.IssueEnrichmentStatus(strings.TrimSpace(status))
		item.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
		item.ObservedAt = time.Unix(0, observedAtUnixNano).UTC()
		if leaseExpiresAtUnixNano > 0 {
			leaseExpiresAt := time.Unix(0, leaseExpiresAtUnixNano).UTC()
			item.LeaseExpiresAt = &leaseExpiresAt
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enrichment source rows: %w", err)
	}
	return items, nil
}

func deriveEnrichmentBackfill(row enrichmentStateRow) (json.RawMessage, EnrichmentProjection) {
	payload := mustMarshalJSON(map[string]any{
		"inferred":               true,
		"status":                 string(row.Status),
		"error":                  row.Error,
		"attemptCount":           row.AttemptCount,
		"leaseExpiresAtUnixNano": leaseUnixNano(row.LeaseExpiresAt),
	})

	projection := EnrichmentProjection{
		IssueID:        row.IssueID,
		Status:         row.Status,
		Error:          row.Error,
		TargetSequence: row.TargetSequence,
		AttemptCount:   row.AttemptCount,
		LeaseExpiresAt: row.LeaseExpiresAt,
		UpdatedAt:      row.ObservedAt,
	}

	return payload, projection
}

func replaceEnrichmentEvents(
	ctx context.Context,
	tx *sql.Tx,
	row enrichmentStateRow,
	payload json.RawMessage,
) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM issue_enrichment_events WHERE issue_id = $1`, row.IssueID); err != nil {
		return fmt.Errorf("delete enrichment events for %q: %w", row.IssueID, err)
	}

	eventID := "issue-enrichment-event:" + row.IssueID + ":observed"
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO issue_enrichment_events (
		     id,
		     issue_id,
		     target_sequence,
		     kind,
		     created_by,
		     created_at_unix_nano,
		     payload_json
		 ) VALUES ($1, $2, $3, 'observed_state', 'system', $4, $5::jsonb)`,
		eventID,
		row.IssueID,
		row.TargetSequence,
		row.ObservedAt.UTC().UnixNano(),
		[]byte(payload),
	); err != nil {
		return fmt.Errorf("insert enrichment event for %q: %w", row.IssueID, err)
	}
	return nil
}

func upsertEnrichmentProjection(ctx context.Context, db issuesdbLike, projection EnrichmentProjection) error {
	leaseExpiresAtUnixNano := leaseUnixNano(projection.LeaseExpiresAt)
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
		projection.IssueID,
		string(projection.Status),
		projection.Error,
		projection.TargetSequence,
		projection.AttemptCount,
		leaseExpiresAtUnixNano,
		"issue-enrichment-event:"+projection.IssueID+":observed",
		projection.UpdatedAt.UTC().UnixNano(),
	); err != nil {
		return fmt.Errorf("upsert enrichment projection for %q: %w", projection.IssueID, err)
	}
	return nil
}

type issuesdbLike interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *EnrichmentStore) loadEnrichmentProjections(ctx context.Context) (map[string]EnrichmentProjection, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT issue_id,
		        status,
		        error,
		        target_sequence,
		        attempt_count,
		        lease_expires_at_unix_nano,
		        latest_event_id,
		        updated_at_unix_nano
		 FROM issue_enrichment_projections`,
	)
	if err != nil {
		return nil, fmt.Errorf("list enrichment projections: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	items := make(map[string]EnrichmentProjection)
	for rows.Next() {
		var (
			item                   EnrichmentProjection
			status                 string
			leaseExpiresAtUnixNano int64
			updatedAtUnixNano      int64
		)
		if err := rows.Scan(
			&item.IssueID,
			&status,
			&item.Error,
			&item.TargetSequence,
			&item.AttemptCount,
			&leaseExpiresAtUnixNano,
			&item.LatestEventID,
			&updatedAtUnixNano,
		); err != nil {
			return nil, fmt.Errorf("scan enrichment projection: %w", err)
		}
		item.Status = issues.IssueEnrichmentStatus(strings.TrimSpace(status))
		item.UpdatedAt = time.Unix(0, updatedAtUnixNano).UTC()
		if leaseExpiresAtUnixNano > 0 {
			leaseExpiresAt := time.Unix(0, leaseExpiresAtUnixNano).UTC()
			item.LeaseExpiresAt = &leaseExpiresAt
		}
		items[item.IssueID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enrichment projections: %w", err)
	}
	return items, nil
}

func compareEnrichmentProjection(row enrichmentStateRow, projection EnrichmentProjection) []string {
	fields := make([]string, 0, 5)
	if row.Status != projection.Status {
		fields = append(fields, "status")
	}
	if strings.TrimSpace(row.Error) != strings.TrimSpace(projection.Error) {
		fields = append(fields, "error")
	}
	if row.TargetSequence != projection.TargetSequence {
		fields = append(fields, "target_sequence")
	}
	if row.AttemptCount != projection.AttemptCount {
		fields = append(fields, "attempt_count")
	}
	if leaseUnixNano(row.LeaseExpiresAt) != leaseUnixNano(projection.LeaseExpiresAt) {
		fields = append(fields, "lease_expires_at")
	}
	return fields
}

func leaseUnixNano(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.UTC().UnixNano()
}
