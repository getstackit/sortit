package appendonly

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	defaultIssueContentCheckpoint = "issue-content-backfill"
	issueContentDomain            = "issue-content"
)

type IssueContentBackfillResult struct {
	Checkpoint       string `json:"checkpoint"`
	ProcessedIssues  int    `json:"processedIssues"`
	WroteEvents      int    `json:"wroteEvents"`
	WroteProjections int    `json:"wroteProjections"`
	Complete         bool   `json:"complete"`
}

type IssueContentParityMismatch struct {
	IssueID string   `json:"issueId"`
	Fields  []string `json:"fields"`
}

type IssueContentParityResult struct {
	Domain             string                       `json:"domain"`
	TotalIssues        int                          `json:"totalIssues"`
	ProjectionCount    int                          `json:"projectionCount"`
	MissingProjections int                          `json:"missingProjections"`
	MismatchedIssues   int                          `json:"mismatchedIssues"`
	Mismatches         []IssueContentParityMismatch `json:"mismatches,omitempty"`
}

type IssueContentStore struct {
	db *sql.DB
}

type issueContentRow struct {
	IssueID   string
	Raw       string
	Tags      []string
	TagScores json.RawMessage
	Embedding []float64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type issueContentEventRow struct {
	ID        string
	IssueID   string
	Sequence  int64
	CreatedAt time.Time
	Payload   json.RawMessage
}

type issueContentProjection struct {
	IssueID     string
	Raw         string
	TagsJSON    json.RawMessage
	TagScores   json.RawMessage
	Embedding   []float64
	LastEventID string
	EventCount  int64
	UpdatedAt   time.Time
}

type issueContentPayload struct {
	Raw       string          `json:"raw"`
	Tags      []string        `json:"tags"`
	TagScores json.RawMessage `json:"tagScores"`
	Embedding []float64       `json:"embedding,omitempty"`
}

func NewIssueContentStore(db *sql.DB) *IssueContentStore {
	return &IssueContentStore{db: db}
}

func (s *IssueContentStore) BackfillBatch(ctx context.Context, checkpointName string, _ int) (IssueContentBackfillResult, error) {
	checkpointName = strings.TrimSpace(checkpointName)
	if checkpointName == "" {
		checkpointName = defaultIssueContentCheckpoint
	}

	rows, err := s.listIssueContentRows(ctx)
	if err != nil {
		return IssueContentBackfillResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IssueContentBackfillResult{}, fmt.Errorf("begin issue content backfill tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	nextSequence, err := loadCurrentIssueContentSequences(ctx, tx)
	if err != nil {
		return IssueContentBackfillResult{}, err
	}

	result := IssueContentBackfillResult{
		Checkpoint:      checkpointName,
		ProcessedIssues: len(rows),
		Complete:        true,
	}

	for _, row := range rows {
		sequence := nextSequence[row.IssueID] + 1
		wrote, err := insertBackfillIssueContentEvent(ctx, tx, row, sequence)
		if err != nil {
			return IssueContentBackfillResult{}, err
		}
		if wrote {
			result.WroteEvents++
			nextSequence[row.IssueID] = sequence
		}
	}

	projections, err := rebuildIssueContentProjections(ctx, tx)
	if err != nil {
		return IssueContentBackfillResult{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM issue_content_projections WHERE NOT EXISTS (SELECT 1 FROM issue_content_facts WHERE issue_id = issue_content_projections.issue_id)`); err != nil {
		return IssueContentBackfillResult{}, fmt.Errorf("delete stale issue content projections: %w", err)
	}
	for _, projection := range projections {
		if err := upsertReducedIssueContentProjection(ctx, tx, projection); err != nil {
			return IssueContentBackfillResult{}, err
		}
		result.WroteProjections++
	}

	if err := tx.Commit(); err != nil {
		return IssueContentBackfillResult{}, fmt.Errorf("commit issue content backfill tx: %w", err)
	}

	framework := NewFrameworkStore(s.db)
	if err := framework.UpsertCheckpoint(ctx, Checkpoint{
		Name:        checkpointName,
		Phase:       "completed",
		CursorJSON:  mustMarshalJSON(struct{}{}),
		SummaryJSON: mustMarshalJSON(result),
	}); err != nil {
		return IssueContentBackfillResult{}, err
	}

	return result, nil
}

func (s *IssueContentStore) CheckParity(ctx context.Context, detailLimit int) (IssueContentParityResult, error) {
	if detailLimit <= 0 {
		detailLimit = 20
	}

	rows, err := s.listIssueContentRows(ctx)
	if err != nil {
		return IssueContentParityResult{}, err
	}
	projections, err := s.listIssueContentProjections(ctx)
	if err != nil {
		return IssueContentParityResult{}, err
	}

	byID := make(map[string]issueContentProjection, len(projections))
	for _, projection := range projections {
		byID[projection.IssueID] = projection
	}

	result := IssueContentParityResult{
		Domain:          issueContentDomain,
		TotalIssues:     len(rows),
		ProjectionCount: len(projections),
	}

	for _, row := range rows {
		projection, ok := byID[row.IssueID]
		if !ok {
			result.MissingProjections++
			result.MismatchedIssues++
			if len(result.Mismatches) < detailLimit {
				result.Mismatches = append(result.Mismatches, IssueContentParityMismatch{
					IssueID: row.IssueID,
					Fields:  []string{"missing_projection"},
				})
			}
			continue
		}

		fields := compareIssueContentRow(row, projection)
		if len(fields) == 0 {
			continue
		}
		result.MismatchedIssues++
		if len(result.Mismatches) < detailLimit {
			result.Mismatches = append(result.Mismatches, IssueContentParityMismatch{
				IssueID: row.IssueID,
				Fields:  fields,
			})
		}
	}

	return result, nil
}

func (s *IssueContentStore) listIssueContentRows(ctx context.Context) ([]issueContentRow, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, raw, tags_json, tag_scores_json, COALESCE(embedding_vector::text, ''), created_at_unix_nano, created_at_unix_nano
		 FROM issues
		 ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list issue content rows: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []issueContentRow
	for rows.Next() {
		var (
			item          issueContentRow
			tagsJSON      json.RawMessage
			embeddingText string
			createdAtNano int64
			updatedAtNano int64
		)
		if err := rows.Scan(&item.IssueID, &item.Raw, &tagsJSON, &item.TagScores, &embeddingText, &createdAtNano, &updatedAtNano); err != nil {
			return nil, fmt.Errorf("scan issue content row: %w", err)
		}
		var tags []string
		if err := json.Unmarshal(tagsJSON, &tags); err != nil {
			return nil, fmt.Errorf("decode issue content tags for %q: %w", item.IssueID, err)
		}
		embedding, err := parseEmbeddingText(embeddingText)
		if err != nil {
			return nil, fmt.Errorf("decode issue content embedding for %q: %w", item.IssueID, err)
		}
		item.Raw = strings.TrimSpace(item.Raw)
		item.Tags = tags
		item.Embedding = embedding
		item.CreatedAt = time.Unix(0, createdAtNano).UTC()
		item.UpdatedAt = time.Unix(0, updatedAtNano).UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue content rows: %w", err)
	}
	return items, nil
}

func loadCurrentIssueContentSequences(ctx context.Context, tx *sql.Tx) (map[string]int64, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT issue_id, COALESCE(MAX(sequence), 0)
		 FROM issue_content_facts
		 GROUP BY issue_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list current issue content sequences: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]int64)
	for rows.Next() {
		var (
			issueID string
			maxSeq  int64
		)
		if err := rows.Scan(&issueID, &maxSeq); err != nil {
			return nil, fmt.Errorf("scan issue content sequence: %w", err)
		}
		result[issueID] = maxSeq
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue content sequences: %w", err)
	}
	return result, nil
}

func insertBackfillIssueContentEvent(ctx context.Context, tx *sql.Tx, row issueContentRow, sequence int64) (bool, error) {
	payload, err := json.Marshal(issueContentPayload{
		Raw:       row.Raw,
		Tags:      append([]string(nil), row.Tags...),
		TagScores: row.TagScores,
		Embedding: append([]float64(nil), row.Embedding...),
	})
	if err != nil {
		return false, fmt.Errorf("marshal issue content payload: %w", err)
	}

	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO issue_content_facts (
		     id,
		     issue_id,
		     sequence,
		     kind,
		     created_by,
		     created_at_unix_nano,
		     payload_json,
		     source,
		     source_id,
		     inferred
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)
		 ON CONFLICT (source, source_id) DO NOTHING`,
		"backfill-issue-content:"+row.IssueID,
		row.IssueID,
		sequence,
		"observed_state",
		"",
		row.UpdatedAt.UTC().UnixNano(),
		payload,
		"legacy_issue_content",
		row.IssueID,
		true,
	)
	if err != nil {
		return false, fmt.Errorf("insert backfill issue content event for %q: %w", row.IssueID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read backfill issue content rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

func rebuildIssueContentProjections(ctx context.Context, tx *sql.Tx) ([]issueContentProjection, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT id, issue_id, sequence, created_at_unix_nano, payload_json
		 FROM issue_content_facts
		 ORDER BY issue_id ASC, sequence ASC, created_at_unix_nano ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list issue content facts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	eventsByIssue := make(map[string][]issueContentEventRow)
	for rows.Next() {
		var (
			item          issueContentEventRow
			createdAtNano int64
		)
		if err := rows.Scan(&item.ID, &item.IssueID, &item.Sequence, &createdAtNano, &item.Payload); err != nil {
			return nil, fmt.Errorf("scan issue content fact: %w", err)
		}
		item.CreatedAt = time.Unix(0, createdAtNano).UTC()
		eventsByIssue[item.IssueID] = append(eventsByIssue[item.IssueID], item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue content facts: %w", err)
	}

	issueIDs := make([]string, 0, len(eventsByIssue))
	for issueID := range eventsByIssue {
		issueIDs = append(issueIDs, issueID)
	}
	sort.Strings(issueIDs)

	out := make([]issueContentProjection, 0, len(issueIDs))
	for _, issueID := range issueIDs {
		var projection issueContentProjection
		projection.IssueID = issueID
		for _, event := range eventsByIssue[issueID] {
			var payload issueContentPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, fmt.Errorf("decode issue content payload for %q: %w", issueID, err)
			}
			tags := payload.Tags
			if tags == nil {
				tags = []string{}
			}
			tagsJSON, err := marshalJSON(tags)
			if err != nil {
				return nil, err
			}
			projection.Raw = strings.TrimSpace(payload.Raw)
			projection.TagsJSON = tagsJSON
			projection.TagScores = append(json.RawMessage(nil), payload.TagScores...)
			projection.Embedding = append([]float64(nil), payload.Embedding...)
			projection.LastEventID = event.ID
			projection.EventCount++
			projection.UpdatedAt = event.CreatedAt
		}
		out = append(out, projection)
	}
	return out, nil
}

func upsertReducedIssueContentProjection(ctx context.Context, tx *sql.Tx, projection issueContentProjection) error {
	embeddingVector, err := formatVectorLiteral(projection.Embedding)
	if err != nil {
		return fmt.Errorf("format issue content projection embedding: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO issue_content_projections (
		     issue_id,
		     raw,
		     tags_json,
		     tag_scores_json,
		     embedding_vector,
		     last_event_id,
		     event_count,
		     updated_at_unix_nano
		 ) VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::vector, $6, $7, $8)
		 ON CONFLICT (issue_id) DO UPDATE
		 SET raw = EXCLUDED.raw,
		     tags_json = EXCLUDED.tags_json,
		     tag_scores_json = EXCLUDED.tag_scores_json,
		     embedding_vector = EXCLUDED.embedding_vector,
		     last_event_id = EXCLUDED.last_event_id,
		     event_count = EXCLUDED.event_count,
		     updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		projection.IssueID,
		projection.Raw,
		[]byte(projection.TagsJSON),
		[]byte(projection.TagScores),
		embeddingVector,
		projection.LastEventID,
		projection.EventCount,
		projection.UpdatedAt.UTC().UnixNano(),
	); err != nil {
		return fmt.Errorf("upsert issue content projection for %q: %w", projection.IssueID, err)
	}
	return nil
}

func (s *IssueContentStore) listIssueContentProjections(ctx context.Context) ([]issueContentProjection, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT issue_id, raw, tags_json, tag_scores_json, COALESCE(embedding_vector::text, ''), last_event_id, event_count, updated_at_unix_nano
		 FROM issue_content_projections
		 ORDER BY issue_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list issue content projections: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []issueContentProjection
	for rows.Next() {
		var (
			item          issueContentProjection
			embeddingText string
			updatedAtNano int64
		)
		if err := rows.Scan(&item.IssueID, &item.Raw, &item.TagsJSON, &item.TagScores, &embeddingText, &item.LastEventID, &item.EventCount, &updatedAtNano); err != nil {
			return nil, fmt.Errorf("scan issue content projection: %w", err)
		}
		embedding, err := parseEmbeddingText(embeddingText)
		if err != nil {
			return nil, fmt.Errorf("decode issue content projection embedding for %q: %w", item.IssueID, err)
		}
		item.Raw = strings.TrimSpace(item.Raw)
		item.Embedding = embedding
		item.UpdatedAt = time.Unix(0, updatedAtNano).UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue content projections: %w", err)
	}
	return items, nil
}

func compareIssueContentRow(row issueContentRow, projection issueContentProjection) []string {
	var fields []string
	if row.Raw != projection.Raw {
		fields = append(fields, "raw")
	}
	var actualTags []string
	if err := json.Unmarshal(projection.TagsJSON, &actualTags); err != nil || !equalStringSlices(row.Tags, actualTags) {
		fields = append(fields, "tags")
	}
	if string(row.TagScores) != string(projection.TagScores) {
		fields = append(fields, "tag_scores")
	}
	if !equalEmbeddings(row.Embedding, projection.Embedding) {
		fields = append(fields, "embedding")
	}
	return fields
}

func marshalJSON(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
