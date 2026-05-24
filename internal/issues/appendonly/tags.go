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
	defaultTagCheckpoint      = "tag-backfill"
	tagDomain                 = "tags"
	tagProjectionStatusActive = "active"
	tagProjectionStatusMerged = "merged"
)

type TagBackfillResult struct {
	Checkpoint       string `json:"checkpoint"`
	ProcessedTags    int    `json:"processedTags"`
	ProcessedMerges  int    `json:"processedMerges"`
	WroteEvents      int    `json:"wroteEvents"`
	WroteProjections int    `json:"wroteProjections"`
	Complete         bool   `json:"complete"`
}

type TagParityMismatch struct {
	TagName string   `json:"tagName"`
	Fields  []string `json:"fields"`
}

type TagParityResult struct {
	Domain                string              `json:"domain"`
	LegacyTagCount        int                 `json:"legacyTagCount"`
	ActiveProjectionCount int                 `json:"activeProjectionCount"`
	MergedAliasCount      int                 `json:"mergedAliasCount"`
	MissingTags           int                 `json:"missingTags"`
	MismatchedTags        int                 `json:"mismatchedTags"`
	MissingMergedAliases  int                 `json:"missingMergedAliases"`
	Mismatches            []TagParityMismatch `json:"mismatches,omitempty"`
}

type TagStore struct {
	db *sql.DB
}

type tagLegacyRow struct {
	Name                  string
	Description           string
	CreatedAt             time.Time
	Embedding             []float64
	Specificity           *float64
	SpecificityLLM        *float64
	SpecificityEmbedding  *float64
	SpecificityComputedAt *time.Time
}

type tagMergeHistoryRow struct {
	ID            int64
	CanonicalName string
	AliasName     string
	MergedAt      time.Time
	MergedBy      string
}

type tagEventPayload struct {
	Description           *string    `json:"description,omitempty"`
	CreatedAtUnixNano     *int64     `json:"createdAtUnixNano,omitempty"`
	Embedding             *[]float64 `json:"embedding,omitempty"`
	Specificity           *float64   `json:"specificity,omitempty"`
	SpecificityLLM        *float64   `json:"specificityLlm,omitempty"`
	SpecificityEmbedding  *float64   `json:"specificityEmbedding,omitempty"`
	SpecificityComputedAt *time.Time `json:"specificityComputedAt,omitempty"`
	CanonicalName         string     `json:"canonicalName,omitempty"`
}

type tagEventRow struct {
	ID        string
	TagName   string
	Sequence  int64
	Kind      string
	CreatedBy string
	CreatedAt time.Time
	Payload   json.RawMessage
	Inferred  bool
}

type tagProjectionState struct {
	Name                  string
	Description           string
	CreatedAt             time.Time
	Embedding             []float64
	Specificity           *float64
	SpecificityLLM        *float64
	SpecificityEmbedding  *float64
	SpecificityComputedAt *time.Time
	Status                string
	CanonicalName         string
	LastEventID           string
	EventCount            int64
	UpdatedAt             time.Time
}

func NewTagStore(db *sql.DB) *TagStore {
	return &TagStore{db: db}
}

func (s *TagStore) BackfillBatch(ctx context.Context, checkpointName string, _ int) (TagBackfillResult, error) {
	checkpointName = strings.TrimSpace(checkpointName)
	if checkpointName == "" {
		checkpointName = defaultTagCheckpoint
	}

	legacyTags, err := s.listLegacyTags(ctx)
	if err != nil {
		return TagBackfillResult{}, err
	}
	mergeRows, err := s.listMergeHistory(ctx)
	if err != nil {
		return TagBackfillResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TagBackfillResult{}, fmt.Errorf("begin tag backfill tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	result := TagBackfillResult{
		Checkpoint:      checkpointName,
		ProcessedTags:   len(legacyTags),
		ProcessedMerges: len(mergeRows),
		Complete:        true,
	}

	nextSequence, err := loadCurrentTagEventSequences(ctx, tx)
	if err != nil {
		return TagBackfillResult{}, err
	}

	legacySet := make(map[string]struct{}, len(legacyTags))
	for _, row := range legacyTags {
		legacySet[row.Name] = struct{}{}
		sequence := nextSequence[row.Name] + 1
		wrote, err := upsertBackfillTagObservedEvent(ctx, tx, row, sequence)
		if err != nil {
			return TagBackfillResult{}, err
		}
		if wrote {
			result.WroteEvents++
			nextSequence[row.Name] = sequence
		}
	}

	sort.SliceStable(mergeRows, func(i, j int) bool {
		if mergeRows[i].MergedAt.Equal(mergeRows[j].MergedAt) {
			return mergeRows[i].ID < mergeRows[j].ID
		}
		return mergeRows[i].MergedAt.Before(mergeRows[j].MergedAt)
	})
	canonicalSeeded := make(map[string]struct{})
	for _, row := range mergeRows {
		if _, ok := legacySet[row.CanonicalName]; !ok {
			if _, ok := canonicalSeeded[row.CanonicalName]; !ok {
				sequence := nextSequence[row.CanonicalName] + 1
				wrote, err := upsertBackfillCanonicalObservedEvent(ctx, tx, row, sequence)
				if err != nil {
					return TagBackfillResult{}, err
				}
				if wrote {
					result.WroteEvents++
					nextSequence[row.CanonicalName] = sequence
				}
				canonicalSeeded[row.CanonicalName] = struct{}{}
			}
		}

		sequence := nextSequence[row.AliasName] + 1
		wrote, err := upsertBackfillTagMergeEvent(ctx, tx, row, sequence)
		if err != nil {
			return TagBackfillResult{}, err
		}
		if wrote {
			result.WroteEvents++
			nextSequence[row.AliasName] = sequence
		}
	}

	eventsByTag, err := loadAllTagEvents(ctx, tx)
	if err != nil {
		return TagBackfillResult{}, err
	}
	projections, err := reduceTagProjections(eventsByTag)
	if err != nil {
		return TagBackfillResult{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM tag_projections WHERE NOT EXISTS (SELECT 1 FROM tag_events WHERE tag_name = tag_projections.name)`); err != nil {
		return TagBackfillResult{}, fmt.Errorf("delete stale tag projections: %w", err)
	}
	for _, projection := range projections {
		if err := upsertTagProjection(ctx, tx, projection); err != nil {
			return TagBackfillResult{}, err
		}
		result.WroteProjections++
	}

	if err := tx.Commit(); err != nil {
		return TagBackfillResult{}, fmt.Errorf("commit tag backfill tx: %w", err)
	}

	framework := NewFrameworkStore(s.db)
	if err := framework.UpsertCheckpoint(ctx, Checkpoint{
		Name:        checkpointName,
		Phase:       "completed",
		CursorJSON:  mustMarshalJSON(struct{}{}),
		SummaryJSON: mustMarshalJSON(result),
	}); err != nil {
		return TagBackfillResult{}, err
	}

	return result, nil
}

func (s *TagStore) CheckParity(ctx context.Context, detailLimit int) (TagParityResult, error) {
	if detailLimit <= 0 {
		detailLimit = 20
	}

	legacyTags, err := s.listLegacyTags(ctx)
	if err != nil {
		return TagParityResult{}, err
	}
	projections, err := s.listTagProjections(ctx)
	if err != nil {
		return TagParityResult{}, err
	}
	mergeRows, err := s.listMergeHistory(ctx)
	if err != nil {
		return TagParityResult{}, err
	}

	activeByName := make(map[string]tagProjectionState, len(projections))
	mergedByAlias := make(map[string]tagProjectionState, len(projections))
	for _, projection := range projections {
		switch projection.Status {
		case tagProjectionStatusMerged:
			mergedByAlias[projection.Name] = projection
		default:
			activeByName[projection.Name] = projection
		}
	}

	result := TagParityResult{
		Domain:                tagDomain,
		LegacyTagCount:        len(legacyTags),
		ActiveProjectionCount: len(activeByName),
		MergedAliasCount:      len(mergedByAlias),
	}

	for _, legacy := range legacyTags {
		projection, ok := activeByName[legacy.Name]
		if !ok {
			result.MissingTags++
			if len(result.Mismatches) < detailLimit {
				result.Mismatches = append(result.Mismatches, TagParityMismatch{
					TagName: legacy.Name,
					Fields:  []string{"missing_projection"},
				})
			}
			continue
		}
		fields := compareLegacyAndProjection(legacy, projection)
		if len(fields) == 0 {
			continue
		}
		result.MismatchedTags++
		if len(result.Mismatches) < detailLimit {
			result.Mismatches = append(result.Mismatches, TagParityMismatch{
				TagName: legacy.Name,
				Fields:  fields,
			})
		}
	}

	for _, row := range mergeRows {
		projection, ok := mergedByAlias[row.AliasName]
		if !ok || projection.CanonicalName != row.CanonicalName {
			result.MissingMergedAliases++
			if len(result.Mismatches) < detailLimit {
				result.Mismatches = append(result.Mismatches, TagParityMismatch{
					TagName: row.AliasName,
					Fields:  []string{"missing_merged_alias_projection"},
				})
			}
		}
	}

	return result, nil
}

func (s *TagStore) listLegacyTags(ctx context.Context) ([]tagLegacyRow, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT name, description, created_at_unix_nano,
		        COALESCE(embedding_vector::text, ''),
		        specificity, specificity_llm, specificity_embedding, specificity_computed_at
		 FROM tags
		 ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list legacy tags: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []tagLegacyRow
	for rows.Next() {
		var (
			item                 tagLegacyRow
			createdAtUnixNano    int64
			embeddingText        string
			specificity          sql.NullFloat64
			specificityLLM       sql.NullFloat64
			specificityEmbedding sql.NullFloat64
			specificityTime      sql.NullTime
		)
		if err := rows.Scan(
			&item.Name,
			&item.Description,
			&createdAtUnixNano,
			&embeddingText,
			&specificity,
			&specificityLLM,
			&specificityEmbedding,
			&specificityTime,
		); err != nil {
			return nil, fmt.Errorf("scan legacy tag: %w", err)
		}
		embedding, err := parseEmbeddingText(embeddingText)
		if err != nil {
			return nil, fmt.Errorf("decode legacy tag embedding for %q: %w", item.Name, err)
		}
		item.Name = sanitizeTagName(item.Name)
		item.Description = strings.TrimSpace(item.Description)
		item.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
		item.Embedding = embedding
		if specificity.Valid {
			value := specificity.Float64
			item.Specificity = &value
		}
		if specificityLLM.Valid {
			value := specificityLLM.Float64
			item.SpecificityLLM = &value
		}
		if specificityEmbedding.Valid {
			value := specificityEmbedding.Float64
			item.SpecificityEmbedding = &value
		}
		if specificityTime.Valid {
			value := specificityTime.Time.UTC()
			item.SpecificityComputedAt = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legacy tags: %w", err)
	}
	return items, nil
}

func (s *TagStore) listMergeHistory(ctx context.Context) ([]tagMergeHistoryRow, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, canonical_name, alias_name, merged_at, merged_by
		 FROM tag_merge_history
		 ORDER BY merged_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tag merge history: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []tagMergeHistoryRow
	for rows.Next() {
		var item tagMergeHistoryRow
		if err := rows.Scan(&item.ID, &item.CanonicalName, &item.AliasName, &item.MergedAt, &item.MergedBy); err != nil {
			return nil, fmt.Errorf("scan tag merge history: %w", err)
		}
		item.CanonicalName = sanitizeTagName(item.CanonicalName)
		item.AliasName = sanitizeTagName(item.AliasName)
		item.MergedAt = item.MergedAt.UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tag merge history: %w", err)
	}
	return items, nil
}

func listTagProjections(ctx context.Context, db *sql.DB) ([]tagProjectionState, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT name, description, created_at_unix_nano,
		        COALESCE(embedding_vector::text, ''),
		        specificity, specificity_llm, specificity_embedding, specificity_computed_at,
		        status, canonical_name, last_event_id, event_count, updated_at_unix_nano
		 FROM tag_projections
		 ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tag projections: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []tagProjectionState
	for rows.Next() {
		item, err := scanTagProjection(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tag projections: %w", err)
	}
	return items, nil
}

func (s *TagStore) listTagProjections(ctx context.Context) ([]tagProjectionState, error) {
	return listTagProjections(ctx, s.db)
}

func upsertBackfillTagObservedEvent(ctx context.Context, tx *sql.Tx, row tagLegacyRow, sequence int64) (bool, error) {
	payload, err := json.Marshal(tagEventPayload{
		Description:           stringPtr(row.Description),
		CreatedAtUnixNano:     int64Ptr(row.CreatedAt.UTC().UnixNano()),
		Embedding:             embeddingPtr(copyEmbedding(row.Embedding)),
		Specificity:           copyFloat64Ptr(row.Specificity),
		SpecificityLLM:        copyFloat64Ptr(row.SpecificityLLM),
		SpecificityEmbedding:  copyFloat64Ptr(row.SpecificityEmbedding),
		SpecificityComputedAt: copyTimePtr(row.SpecificityComputedAt),
	})
	if err != nil {
		return false, fmt.Errorf("marshal backfill tag observed payload: %w", err)
	}
	return insertBackfillTagEvent(ctx, tx, tagEventRow{
		ID:        "backfill-tag:" + row.Name,
		TagName:   row.Name,
		Sequence:  sequence,
		Kind:      "observed_state",
		CreatedAt: row.CreatedAt.UTC(),
		Payload:   payload,
		Inferred:  true,
	}, "legacy_tag", row.Name)
}

func upsertBackfillCanonicalObservedEvent(ctx context.Context, tx *sql.Tx, row tagMergeHistoryRow, sequence int64) (bool, error) {
	payload, err := json.Marshal(tagEventPayload{
		CreatedAtUnixNano: int64Ptr(row.MergedAt.UTC().UnixNano()),
	})
	if err != nil {
		return false, fmt.Errorf("marshal inferred canonical tag payload: %w", err)
	}
	return insertBackfillTagEvent(ctx, tx, tagEventRow{
		ID:        "backfill-tag-canonical:" + row.CanonicalName,
		TagName:   row.CanonicalName,
		Sequence:  sequence,
		Kind:      "observed_state",
		CreatedBy: row.MergedBy,
		CreatedAt: row.MergedAt.UTC(),
		Payload:   payload,
		Inferred:  true,
	}, "legacy_tag_merge_canonical", row.CanonicalName)
}

func upsertBackfillTagMergeEvent(ctx context.Context, tx *sql.Tx, row tagMergeHistoryRow, sequence int64) (bool, error) {
	payload, err := json.Marshal(tagEventPayload{
		CanonicalName: row.CanonicalName,
	})
	if err != nil {
		return false, fmt.Errorf("marshal tag merge payload: %w", err)
	}
	return insertBackfillTagEvent(ctx, tx, tagEventRow{
		ID:        fmt.Sprintf("backfill-tag-merge:%d", row.ID),
		TagName:   row.AliasName,
		Sequence:  sequence,
		Kind:      "merged",
		CreatedBy: row.MergedBy,
		CreatedAt: row.MergedAt.UTC(),
		Payload:   payload,
	}, "legacy_tag_merge", fmt.Sprintf("%d", row.ID))
}

func insertBackfillTagEvent(ctx context.Context, tx *sql.Tx, event tagEventRow, source string, sourceID string) (bool, error) {
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO tag_events (
		     id,
		     tag_name,
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
		event.ID,
		event.TagName,
		event.Sequence,
		event.Kind,
		event.CreatedBy,
		event.CreatedAt.UTC().UnixNano(),
		[]byte(event.Payload),
		source,
		sourceID,
		event.Inferred,
	)
	if err != nil {
		return false, fmt.Errorf("insert backfill tag event for %q: %w", event.TagName, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read backfill tag event rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

func loadAllTagEvents(ctx context.Context, tx *sql.Tx) (map[string][]tagEventRow, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT id, tag_name, sequence, kind, created_by, created_at_unix_nano, payload_json, inferred
		 FROM tag_events
		 ORDER BY tag_name ASC, sequence ASC, created_at_unix_nano ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tag events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	eventsByTag := make(map[string][]tagEventRow)
	for rows.Next() {
		var (
			item              tagEventRow
			createdAtUnixNano int64
		)
		if err := rows.Scan(&item.ID, &item.TagName, &item.Sequence, &item.Kind, &item.CreatedBy, &createdAtUnixNano, &item.Payload, &item.Inferred); err != nil {
			return nil, fmt.Errorf("scan tag event: %w", err)
		}
		item.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
		eventsByTag[item.TagName] = append(eventsByTag[item.TagName], item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tag events: %w", err)
	}
	return eventsByTag, nil
}

func loadCurrentTagEventSequences(ctx context.Context, tx *sql.Tx) (map[string]int64, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT tag_name, COALESCE(MAX(sequence), 0)
		 FROM tag_events
		 GROUP BY tag_name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list current tag event sequences: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]int64)
	for rows.Next() {
		var (
			tagName string
			maxSeq  int64
		)
		if err := rows.Scan(&tagName, &maxSeq); err != nil {
			return nil, fmt.Errorf("scan current tag event sequence: %w", err)
		}
		result[tagName] = maxSeq
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current tag event sequences: %w", err)
	}
	return result, nil
}

func reduceTagProjections(eventsByTag map[string][]tagEventRow) ([]tagProjectionState, error) {
	names := make([]string, 0, len(eventsByTag))
	for name := range eventsByTag {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]tagProjectionState, 0, len(names))
	for _, name := range names {
		projection, err := reduceTagProjection(name, eventsByTag[name])
		if err != nil {
			return nil, err
		}
		out = append(out, projection)
	}
	return out, nil
}

func reduceTagProjection(name string, events []tagEventRow) (tagProjectionState, error) {
	projection := tagProjectionState{
		Name:   name,
		Status: tagProjectionStatusActive,
	}

	for _, event := range events {
		var payload tagEventPayload
		if len(event.Payload) > 0 {
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return tagProjectionState{}, fmt.Errorf("decode tag event payload for %q: %w", name, err)
			}
		}
		switch event.Kind {
		case "observed_state", "upserted":
			if payload.CreatedAtUnixNano != nil && projection.CreatedAt.IsZero() {
				projection.CreatedAt = time.Unix(0, *payload.CreatedAtUnixNano).UTC()
			}
			if projection.CreatedAt.IsZero() {
				projection.CreatedAt = event.CreatedAt.UTC()
			}
			if payload.Description != nil {
				projection.Description = strings.TrimSpace(*payload.Description)
			}
			if payload.Embedding != nil {
				projection.Embedding = copyEmbedding(*payload.Embedding)
			}
			if payload.Specificity != nil {
				projection.Specificity = copyFloat64Ptr(payload.Specificity)
			}
			if payload.SpecificityLLM != nil {
				projection.SpecificityLLM = copyFloat64Ptr(payload.SpecificityLLM)
			}
			if payload.SpecificityEmbedding != nil {
				projection.SpecificityEmbedding = copyFloat64Ptr(payload.SpecificityEmbedding)
			}
			if payload.SpecificityComputedAt != nil {
				value := payload.SpecificityComputedAt.UTC()
				projection.SpecificityComputedAt = &value
			}
			if projection.Status == "" {
				projection.Status = tagProjectionStatusActive
			}
		case "specificity_updated":
			projection.Specificity = copyFloat64Ptr(payload.Specificity)
			projection.SpecificityLLM = copyFloat64Ptr(payload.SpecificityLLM)
			projection.SpecificityEmbedding = copyFloat64Ptr(payload.SpecificityEmbedding)
			projection.SpecificityComputedAt = copyTimePtr(payload.SpecificityComputedAt)
			if projection.CreatedAt.IsZero() {
				projection.CreatedAt = event.CreatedAt.UTC()
			}
			if projection.Status == "" {
				projection.Status = tagProjectionStatusActive
			}
		case "merged":
			if projection.CreatedAt.IsZero() {
				projection.CreatedAt = event.CreatedAt.UTC()
			}
			projection.Status = tagProjectionStatusMerged
			projection.CanonicalName = sanitizeTagName(payload.CanonicalName)
		default:
			if projection.CreatedAt.IsZero() {
				projection.CreatedAt = event.CreatedAt.UTC()
			}
		}
		projection.LastEventID = event.ID
		projection.EventCount++
		projection.UpdatedAt = event.CreatedAt.UTC()
	}

	if projection.CreatedAt.IsZero() {
		projection.CreatedAt = projection.UpdatedAt
	}
	if projection.Status == "" {
		projection.Status = tagProjectionStatusActive
	}
	return projection, nil
}

func upsertTagProjection(ctx context.Context, tx *sql.Tx, projection tagProjectionState) error {
	vectorLiteral, err := formatVectorLiteral(projection.Embedding)
	if err != nil {
		return fmt.Errorf("format tag projection embedding: %w", err)
	}

	var (
		specificity           any
		specificityLLM        any
		specificityEmbedding  any
		specificityComputedAt any
	)
	if projection.Specificity != nil {
		specificity = *projection.Specificity
	}
	if projection.SpecificityLLM != nil {
		specificityLLM = *projection.SpecificityLLM
	}
	if projection.SpecificityEmbedding != nil {
		specificityEmbedding = *projection.SpecificityEmbedding
	}
	if projection.SpecificityComputedAt != nil {
		specificityComputedAt = projection.SpecificityComputedAt.UTC()
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO tag_projections (
		     name,
		     description,
		     created_at_unix_nano,
		     embedding_vector,
		     specificity,
		     specificity_llm,
		     specificity_embedding,
		     specificity_computed_at,
		     status,
		     canonical_name,
		     last_event_id,
		     event_count,
		     updated_at_unix_nano
		 ) VALUES ($1, $2, $3, $4::vector, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT (name) DO UPDATE
		 SET description = EXCLUDED.description,
		     created_at_unix_nano = EXCLUDED.created_at_unix_nano,
		     embedding_vector = EXCLUDED.embedding_vector,
		     specificity = EXCLUDED.specificity,
		     specificity_llm = EXCLUDED.specificity_llm,
		     specificity_embedding = EXCLUDED.specificity_embedding,
		     specificity_computed_at = EXCLUDED.specificity_computed_at,
		     status = EXCLUDED.status,
		     canonical_name = EXCLUDED.canonical_name,
		     last_event_id = EXCLUDED.last_event_id,
		     event_count = EXCLUDED.event_count,
		     updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		projection.Name,
		projection.Description,
		projection.CreatedAt.UTC().UnixNano(),
		vectorLiteral,
		specificity,
		specificityLLM,
		specificityEmbedding,
		specificityComputedAt,
		projection.Status,
		projection.CanonicalName,
		projection.LastEventID,
		projection.EventCount,
		projection.UpdatedAt.UTC().UnixNano(),
	); err != nil {
		return fmt.Errorf("upsert reduced tag projection %q: %w", projection.Name, err)
	}
	return nil
}

func compareLegacyAndProjection(legacy tagLegacyRow, projection tagProjectionState) []string {
	var fields []string
	if projection.Status != tagProjectionStatusActive {
		fields = append(fields, "status")
	}
	if strings.TrimSpace(projection.Description) != strings.TrimSpace(legacy.Description) {
		fields = append(fields, "description")
	}
	if !projection.CreatedAt.Equal(legacy.CreatedAt) {
		fields = append(fields, "created_at")
	}
	if !equalEmbeddings(projection.Embedding, legacy.Embedding) {
		fields = append(fields, "embedding")
	}
	if !equalFloatPtr(projection.Specificity, legacy.Specificity) {
		fields = append(fields, "specificity")
	}
	if !equalFloatPtr(projection.SpecificityLLM, legacy.SpecificityLLM) {
		fields = append(fields, "specificity_llm")
	}
	if !equalFloatPtr(projection.SpecificityEmbedding, legacy.SpecificityEmbedding) {
		fields = append(fields, "specificity_embedding")
	}
	if !equalTimePtr(projection.SpecificityComputedAt, legacy.SpecificityComputedAt) {
		fields = append(fields, "specificity_computed_at")
	}
	return fields
}

func scanTagProjection(scanner interface{ Scan(dest ...any) error }) (tagProjectionState, error) {
	var (
		item                 tagProjectionState
		createdAtUnixNano    int64
		updatedAtUnixNano    int64
		embeddingText        string
		specificity          sql.NullFloat64
		specificityLLM       sql.NullFloat64
		specificityEmbedding sql.NullFloat64
		specificityTime      sql.NullTime
	)
	if err := scanner.Scan(
		&item.Name,
		&item.Description,
		&createdAtUnixNano,
		&embeddingText,
		&specificity,
		&specificityLLM,
		&specificityEmbedding,
		&specificityTime,
		&item.Status,
		&item.CanonicalName,
		&item.LastEventID,
		&item.EventCount,
		&updatedAtUnixNano,
	); err != nil {
		return tagProjectionState{}, fmt.Errorf("scan tag projection: %w", err)
	}
	embedding, err := parseEmbeddingText(embeddingText)
	if err != nil {
		return tagProjectionState{}, fmt.Errorf("decode tag projection embedding for %q: %w", item.Name, err)
	}
	item.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
	item.UpdatedAt = time.Unix(0, updatedAtUnixNano).UTC()
	item.Embedding = embedding
	if specificity.Valid {
		value := specificity.Float64
		item.Specificity = &value
	}
	if specificityLLM.Valid {
		value := specificityLLM.Float64
		item.SpecificityLLM = &value
	}
	if specificityEmbedding.Valid {
		value := specificityEmbedding.Float64
		item.SpecificityEmbedding = &value
	}
	if specificityTime.Valid {
		value := specificityTime.Time.UTC()
		item.SpecificityComputedAt = &value
	}
	return item, nil
}

func parseEmbeddingText(text string) ([]float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	var result []float64
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func formatVectorLiteral(values []float64) (any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return string(encoded), nil
}

func sanitizeTagName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func copyEmbedding(values []float64) []float64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]float64, len(values))
	copy(out, values)
	return out
}

func copyFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	next := *value
	return &next
}

func copyTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	next := value.UTC()
	return &next
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func embeddingPtr(value []float64) *[]float64 {
	if len(value) == 0 {
		return nil
	}
	return &value
}

func equalFloatPtr(left, right *float64) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
}

func equalTimePtr(left, right *time.Time) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return left.UTC().Equal(right.UTC())
	}
}

func equalEmbeddings(left, right []float64) bool {
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
