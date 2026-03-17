package issues

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	tagProjectionStatusActive = "active"
	tagProjectionStatusMerged = "merged"
)

type tagEventRecord struct {
	ID        string
	TagName   string
	Sequence  int64
	Kind      string
	CreatedBy string
	CreatedAt time.Time
	Payload   json.RawMessage
	Source    string
	SourceID  string
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

type tagUpsertPayload struct {
	Description       *string    `json:"description,omitempty"`
	CreatedAtUnixNano *int64     `json:"createdAtUnixNano,omitempty"`
	Embedding         *[]float64 `json:"embedding,omitempty"`
}

type tagSpecificityPayload struct {
	Specificity           *float64   `json:"specificity,omitempty"`
	SpecificityLLM        *float64   `json:"specificityLlm,omitempty"`
	SpecificityEmbedding  *float64   `json:"specificityEmbedding,omitempty"`
	SpecificityComputedAt *time.Time `json:"specificityComputedAt,omitempty"`
}

type tagMergedPayload struct {
	CanonicalName string `json:"canonicalName"`
}

func appendTagUpsert(ctx context.Context, tx *sql.Tx, tag Tag) error {
	record, err := recordFromTag(tag)
	if err != nil {
		return err
	}
	if record.Name == "" {
		return nil
	}

	state, err := loadTagProjectionState(ctx, tx, record.Name)
	if err != nil {
		return err
	}

	createdAt := time.Unix(0, record.CreatedAtUnixNano).UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	payload, err := marshalJSONB(tagUpsertPayload{
		Description:       stringPtr(record.Description),
		CreatedAtUnixNano: int64Ptr(createdAt.UnixNano()),
		Embedding:         embeddingPtr(copyEmbedding(tag.Embedding)),
	}, tagUpsertPayload{})
	if err != nil {
		return fmt.Errorf("marshal tag upsert payload: %w", err)
	}

	event := tagEventRecord{
		ID:        NewTagEventID(),
		TagName:   record.Name,
		Sequence:  nextTagEventSequence(state),
		Kind:      "upserted",
		CreatedBy: "",
		CreatedAt: createdAt,
		Payload:   payload,
		Source:    "tag_upsert",
		SourceID:  NewTagEventID(),
	}
	if err := insertTagEvent(ctx, tx, event); err != nil {
		return err
	}

	next := mergeTagProjectionUpsert(state, tag, createdAt, event.ID)
	return upsertTagProjection(ctx, tx, next)
}

func appendTagSpecificityUpdate(
	ctx context.Context,
	tx *sql.Tx,
	name string,
	specificity *float64,
	llm *float64,
	embedding *float64,
	computedAt *time.Time,
) error {
	name = sanitizeTagName(name)
	if name == "" {
		return fmt.Errorf("tag name is required")
	}

	state, err := loadTagProjectionState(ctx, tx, name)
	if err != nil {
		return err
	}

	eventTime := time.Now().UTC()
	if computedAt != nil {
		eventTime = computedAt.UTC()
	}

	payload, err := marshalJSONB(tagSpecificityPayload{
		Specificity:           copyFloat64Ptr(specificity),
		SpecificityLLM:        copyFloat64Ptr(llm),
		SpecificityEmbedding:  copyFloat64Ptr(embedding),
		SpecificityComputedAt: copyTimePtr(computedAt),
	}, tagSpecificityPayload{})
	if err != nil {
		return fmt.Errorf("marshal tag specificity payload: %w", err)
	}

	event := tagEventRecord{
		ID:        NewTagEventID(),
		TagName:   name,
		Sequence:  nextTagEventSequence(state),
		Kind:      "specificity_updated",
		CreatedBy: "",
		CreatedAt: eventTime,
		Payload:   payload,
		Source:    "tag_specificity_update",
		SourceID:  NewTagEventID(),
	}
	if err := insertTagEvent(ctx, tx, event); err != nil {
		return err
	}

	next := mergeTagProjectionSpecificity(state, name, specificity, llm, embedding, computedAt, eventTime, event.ID)
	return upsertTagProjection(ctx, tx, next)
}

func ensureActiveTagProjection(ctx context.Context, tx *sql.Tx, name string, createdBy string, createdAt time.Time, source string, sourceID string) (tagProjectionState, error) { //nolint:unparam
	name = sanitizeTagName(name)
	if name == "" {
		return tagProjectionState{}, fmt.Errorf("tag name is required")
	}

	state, err := loadTagProjectionState(ctx, tx, name)
	if err != nil {
		return tagProjectionState{}, err
	}
	if state != nil {
		return *state, nil
	}

	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	payload, err := marshalJSONB(tagUpsertPayload{
		CreatedAtUnixNano: int64Ptr(createdAt.UnixNano()),
	}, tagUpsertPayload{})
	if err != nil {
		return tagProjectionState{}, fmt.Errorf("marshal inferred tag payload: %w", err)
	}

	event := tagEventRecord{
		ID:        NewTagEventID(),
		TagName:   name,
		Sequence:  1,
		Kind:      "observed_state",
		CreatedBy: strings.TrimSpace(createdBy),
		CreatedAt: createdAt,
		Payload:   payload,
		Source:    source,
		SourceID:  sourceID,
		Inferred:  true,
	}
	if err := insertTagEvent(ctx, tx, event); err != nil {
		return tagProjectionState{}, err
	}

	next := tagProjectionState{
		Name:          name,
		CreatedAt:     createdAt,
		Status:        tagProjectionStatusActive,
		LastEventID:   event.ID,
		EventCount:    1,
		UpdatedAt:     createdAt,
		CanonicalName: "",
	}
	if err := upsertTagProjection(ctx, tx, next); err != nil {
		return tagProjectionState{}, err
	}
	return next, nil
}

func appendTagMerge(
	ctx context.Context,
	tx *sql.Tx,
	canonical string,
	alias string,
	mergedBy string,
	mergedAt time.Time,
	source string,
	sourceID string,
) error {
	canonical = sanitizeTagName(canonical)
	alias = sanitizeTagName(alias)
	if canonical == "" || alias == "" {
		return fmt.Errorf("canonical and alias tag names are required")
	}

	if _, err := ensureActiveTagProjection(ctx, tx, canonical, mergedBy, mergedAt, source, sourceID+":canonical"); err != nil {
		return err
	}

	state, err := loadTagProjectionState(ctx, tx, alias)
	if err != nil {
		return err
	}

	payload, err := marshalJSONB(tagMergedPayload{CanonicalName: canonical}, tagMergedPayload{})
	if err != nil {
		return fmt.Errorf("marshal tag merge payload: %w", err)
	}

	event := tagEventRecord{
		ID:        NewTagEventID(),
		TagName:   alias,
		Sequence:  nextTagEventSequence(state),
		Kind:      "merged",
		CreatedBy: strings.TrimSpace(mergedBy),
		CreatedAt: mergedAt.UTC(),
		Payload:   payload,
		Source:    source,
		SourceID:  sourceID,
	}
	if err := insertTagEvent(ctx, tx, event); err != nil {
		return err
	}

	next := mergeTagProjectionMerged(state, alias, canonical, mergedAt.UTC(), event.ID)
	return upsertTagProjection(ctx, tx, next)
}

func loadTagProjectionState(ctx context.Context, tx *sql.Tx, name string) (*tagProjectionState, error) {
	name = sanitizeTagName(name)
	if name == "" {
		return nil, nil
	}

	var (
		state                 tagProjectionState
		embeddingText         string
		createdAtUnixNano     int64
		updatedAtUnixNano     int64
		specificity           sql.NullFloat64
		specificityLLM        sql.NullFloat64
		specificityEmbedding  sql.NullFloat64
		specificityComputedAt sql.NullTime
	)

	err := tx.QueryRowContext(
		ctx,
		`SELECT name, description, created_at_unix_nano,
		        COALESCE(embedding_vector::text, ''),
		        specificity, specificity_llm, specificity_embedding, specificity_computed_at,
		        status, canonical_name, last_event_id, event_count, updated_at_unix_nano
		 FROM tag_projections
		 WHERE name = $1`,
		name,
	).Scan(
		&state.Name,
		&state.Description,
		&createdAtUnixNano,
		&embeddingText,
		&specificity,
		&specificityLLM,
		&specificityEmbedding,
		&specificityComputedAt,
		&state.Status,
		&state.CanonicalName,
		&state.LastEventID,
		&state.EventCount,
		&updatedAtUnixNano,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load tag projection %q: %w", name, err)
	}

	embedding, err := parseEmbeddingText(embeddingText)
	if err != nil {
		return nil, fmt.Errorf("decode embedding for tag projection %q: %w", name, err)
	}

	state.CreatedAt = time.Unix(0, createdAtUnixNano).UTC()
	state.UpdatedAt = time.Unix(0, updatedAtUnixNano).UTC()
	state.Embedding = embedding
	if specificity.Valid {
		state.Specificity = copyFloat64Ptr(&specificity.Float64)
	}
	if specificityLLM.Valid {
		state.SpecificityLLM = copyFloat64Ptr(&specificityLLM.Float64)
	}
	if specificityEmbedding.Valid {
		state.SpecificityEmbedding = copyFloat64Ptr(&specificityEmbedding.Float64)
	}
	if specificityComputedAt.Valid {
		value := specificityComputedAt.Time.UTC()
		state.SpecificityComputedAt = &value
	}
	return &state, nil
}

func insertTagEvent(ctx context.Context, tx *sql.Tx, event tagEventRecord) error {
	payload := event.Payload
	if len(payload) == 0 {
		var err error
		payload, err = marshalJSONB(map[string]any{}, map[string]any{})
		if err != nil {
			return fmt.Errorf("marshal empty tag event payload: %w", err)
		}
	}
	if _, err := tx.ExecContext(
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
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)`,
		strings.TrimSpace(event.ID),
		sanitizeTagName(event.TagName),
		event.Sequence,
		strings.TrimSpace(event.Kind),
		strings.TrimSpace(event.CreatedBy),
		event.CreatedAt.UTC().UnixNano(),
		[]byte(payload),
		strings.TrimSpace(event.Source),
		strings.TrimSpace(event.SourceID),
		event.Inferred,
	); err != nil {
		return fmt.Errorf("insert tag event %q for %q: %w", event.ID, event.TagName, err)
	}
	return nil
}

func upsertTagProjection(ctx context.Context, tx *sql.Tx, state tagProjectionState) error {
	vectorLiteral, err := formatVectorLiteral(state.Embedding)
	if err != nil {
		return fmt.Errorf("format tag projection embedding vector: %w", err)
	}

	var (
		specificity           any
		specificityLLM        any
		specificityEmbedding  any
		specificityComputedAt any
	)
	if state.Specificity != nil {
		specificity = *state.Specificity
	}
	if state.SpecificityLLM != nil {
		specificityLLM = *state.SpecificityLLM
	}
	if state.SpecificityEmbedding != nil {
		specificityEmbedding = *state.SpecificityEmbedding
	}
	if state.SpecificityComputedAt != nil {
		specificityComputedAt = state.SpecificityComputedAt.UTC()
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
		state.Name,
		state.Description,
		state.CreatedAt.UTC().UnixNano(),
		vectorLiteral,
		specificity,
		specificityLLM,
		specificityEmbedding,
		specificityComputedAt,
		state.Status,
		state.CanonicalName,
		state.LastEventID,
		state.EventCount,
		state.UpdatedAt.UTC().UnixNano(),
	); err != nil {
		return fmt.Errorf("upsert tag projection %q: %w", state.Name, err)
	}
	return nil
}

func nextTagEventSequence(state *tagProjectionState) int64 {
	if state == nil || state.EventCount <= 0 {
		return 1
	}
	return state.EventCount + 1
}

func mergeTagProjectionUpsert(state *tagProjectionState, tag Tag, createdAt time.Time, lastEventID string) tagProjectionState {
	next := tagProjectionState{
		Name:          sanitizeTagName(tag.Name),
		Description:   strings.TrimSpace(tag.Description),
		CreatedAt:     createdAt,
		Embedding:     copyEmbedding(tag.Embedding),
		Status:        tagProjectionStatusActive,
		CanonicalName: "",
		LastEventID:   lastEventID,
		EventCount:    1,
		UpdatedAt:     createdAt,
	}
	if state != nil {
		next = *state
		next.LastEventID = lastEventID
		next.EventCount++
		next.UpdatedAt = createdAt
		if next.CreatedAt.IsZero() {
			next.CreatedAt = createdAt
		}
		if next.Status == "" {
			next.Status = tagProjectionStatusActive
		}
	}
	if next.Description == "" && strings.TrimSpace(tag.Description) != "" {
		next.Description = strings.TrimSpace(tag.Description)
	}
	if len(next.Embedding) == 0 && len(tag.Embedding) > 0 {
		next.Embedding = copyEmbedding(tag.Embedding)
	}
	return next
}

func mergeTagProjectionSpecificity(
	state *tagProjectionState,
	name string,
	specificity *float64,
	llm *float64,
	embedding *float64,
	computedAt *time.Time,
	eventTime time.Time,
	lastEventID string,
) tagProjectionState {
	next := tagProjectionState{
		Name:          sanitizeTagName(name),
		CreatedAt:     eventTime,
		Status:        tagProjectionStatusActive,
		CanonicalName: "",
		LastEventID:   lastEventID,
		EventCount:    1,
		UpdatedAt:     eventTime,
	}
	if state != nil {
		next = *state
		next.LastEventID = lastEventID
		next.EventCount++
		next.UpdatedAt = eventTime
		if next.Status == "" {
			next.Status = tagProjectionStatusActive
		}
	}
	next.Specificity = copyFloat64Ptr(specificity)
	next.SpecificityLLM = copyFloat64Ptr(llm)
	next.SpecificityEmbedding = copyFloat64Ptr(embedding)
	next.SpecificityComputedAt = copyTimePtr(computedAt)
	return next
}

func mergeTagProjectionMerged(state *tagProjectionState, alias string, canonical string, mergedAt time.Time, lastEventID string) tagProjectionState {
	next := tagProjectionState{
		Name:          alias,
		CreatedAt:     mergedAt,
		Status:        tagProjectionStatusMerged,
		CanonicalName: canonical,
		LastEventID:   lastEventID,
		EventCount:    1,
		UpdatedAt:     mergedAt,
	}
	if state != nil {
		next = *state
		next.LastEventID = lastEventID
		next.EventCount++
		if next.CreatedAt.IsZero() {
			next.CreatedAt = mergedAt
		}
		next.UpdatedAt = mergedAt
	}
	next.Status = tagProjectionStatusMerged
	next.CanonicalName = canonical
	return next
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
