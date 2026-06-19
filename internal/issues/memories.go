package issues

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"sortit/internal/domain"
)

// ErrMemoryNotFound is returned when a memories row with the requested id
// doesn't exist.
var ErrMemoryNotFound = errors.New("memory not found")

// MemoryListOptions filters a memory listing. A zero value lists every memory,
// most-recent first.
type MemoryListOptions struct {
	// Status, when non-empty, restricts results to memories in that state.
	Status domain.MemoryStatus
	Limit  int
	Offset int
}

// MemorySimilarity is a memory plus its cosine similarity to a query embedding.
type MemorySimilarity struct {
	Memory     domain.Memory
	Similarity float64
}

// MemoryStore is the read/write surface for permanent knowledge artifacts.
// Memories are kept in their own table so they never enter the corpus
// statistics they are derived from; they merely reuse the same vector/tag
// primitives.
type MemoryStore interface {
	ListMemories(ctx context.Context, opts MemoryListOptions) ([]domain.Memory, error)
	GetMemory(ctx context.Context, id string) (domain.Memory, error)
	UpsertMemory(ctx context.Context, memory domain.Memory) error
	DeleteMemory(ctx context.Context, id string) error
	// SearchMemories returns the memories most similar to the query embedding,
	// nearest first. Only active memories with an embedding participate.
	SearchMemories(ctx context.Context, query []float64, limit int) ([]MemorySimilarity, error)
}

const memorySelectColumns = `id, title, body, kind, anchor_tags_json, anchor_region,
	tag_scores_json, COALESCE(embedding_vector::text, ''), status, superseded_by,
	source, source_issue_ids_json, confidence, created_by,
	created_at_unix_nano, updated_at_unix_nano,
	last_reinforced_at_unix_nano, reinforcement_count`

// ListMemories returns memories in the database, most-recent first.
func (s *PostgresStore) ListMemories(ctx context.Context, opts MemoryListOptions) ([]domain.Memory, error) {
	status := strings.TrimSpace(string(opts.Status))
	limit := opts.Limit
	if limit <= 0 {
		limit = 2147483647
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+memorySelectColumns+`
		 FROM memories
		 WHERE ($1 = '' OR status = $1)
		 ORDER BY created_at_unix_nano DESC, id
		 LIMIT $2 OFFSET $3`,
		status, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]domain.Memory, 0)
	for rows.Next() {
		memory, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memories: %w", err)
	}
	return out, nil
}

// GetMemory returns one memory or ErrMemoryNotFound.
func (s *PostgresStore) GetMemory(ctx context.Context, id string) (domain.Memory, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Memory{}, ErrMemoryNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+memorySelectColumns+` FROM memories WHERE id = $1`, id,
	)
	memory, err := scanMemory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Memory{}, ErrMemoryNotFound
	}
	if err != nil {
		return domain.Memory{}, err
	}
	return memory, nil
}

// UpsertMemory inserts or updates a memory by id. CreatedAt/CreatedBy are
// preserved on update.
func (s *PostgresStore) UpsertMemory(ctx context.Context, memory domain.Memory) error {
	record, err := memoryRecordFrom(memory)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO memories
		   (id, title, body, kind, anchor_tags_json, anchor_region, tag_scores_json,
		    embedding_vector, status, superseded_by, source, source_issue_ids_json,
		    confidence, created_by, created_at_unix_nano, updated_at_unix_nano,
		    last_reinforced_at_unix_nano, reinforcement_count)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		 ON CONFLICT (id) DO UPDATE SET
		   title = EXCLUDED.title,
		   body = EXCLUDED.body,
		   kind = EXCLUDED.kind,
		   anchor_tags_json = EXCLUDED.anchor_tags_json,
		   anchor_region = EXCLUDED.anchor_region,
		   tag_scores_json = EXCLUDED.tag_scores_json,
		   embedding_vector = EXCLUDED.embedding_vector,
		   status = EXCLUDED.status,
		   superseded_by = EXCLUDED.superseded_by,
		   source = EXCLUDED.source,
		   source_issue_ids_json = EXCLUDED.source_issue_ids_json,
		   confidence = EXCLUDED.confidence,
		   updated_at_unix_nano = EXCLUDED.updated_at_unix_nano,
		   last_reinforced_at_unix_nano = EXCLUDED.last_reinforced_at_unix_nano,
		   reinforcement_count = EXCLUDED.reinforcement_count`,
		record.id, record.title, record.body, record.kind, record.anchorTagsJSON,
		record.anchorRegion, record.tagScoresJSON, record.embeddingVector, record.status,
		record.supersededBy, record.source, record.sourceIssueIDsJSON, record.confidence,
		record.createdBy, record.createdAtNS, record.updatedAtNS,
		record.lastReinforcedAtNS, record.reinforcementCount,
	)
	if err != nil {
		return fmt.Errorf("upsert memory: %w", err)
	}
	return nil
}

// DeleteMemory removes the row by id. No-op if the id doesn't exist.
func (s *PostgresStore) DeleteMemory(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM memories WHERE id = $1`, strings.TrimSpace(id)); err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}
	return nil
}

// SearchMemories returns active memories ranked by cosine similarity to the
// query embedding, nearest first.
func (s *PostgresStore) SearchMemories(ctx context.Context, query []float64, limit int) ([]MemorySimilarity, error) {
	if len(query) == 0 {
		return []MemorySimilarity{}, nil
	}
	if limit <= 0 {
		limit = 8
	}
	vectorLiteral, err := formatVectorLiteral(query)
	if err != nil {
		return nil, fmt.Errorf("format memory query embedding: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+memorySelectColumns+`,
		        (1 - (embedding_vector <=> $1::vector)) AS similarity
		 FROM memories
		 WHERE status = 'active'
		   AND embedding_vector IS NOT NULL
		   AND vector_dims(embedding_vector) = $2
		 ORDER BY embedding_vector <=> $1::vector ASC, created_at_unix_nano DESC, id ASC
		 LIMIT $3`,
		vectorLiteral, len(query), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]MemorySimilarity, 0, limit)
	for rows.Next() {
		memory, similarity, err := scanMemoryWithSimilarity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, MemorySimilarity{Memory: memory, Similarity: math.Round(similarity*1000) / 1000})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memory similarities: %w", err)
	}
	return out, nil
}

type memoryRow interface {
	Scan(dest ...any) error
}

type memoryRecord struct {
	id                 string
	title              string
	body               string
	kind               string
	anchorTagsJSON     json.RawMessage
	anchorRegion       string
	tagScoresJSON      json.RawMessage
	embeddingVector    any
	status             string
	supersededBy       string
	source             string
	sourceIssueIDsJSON json.RawMessage
	confidence         float64
	createdBy          string
	createdAtNS        int64
	updatedAtNS        int64
	lastReinforcedAtNS int64
	reinforcementCount int64
}

func memoryRecordFrom(memory domain.Memory) (memoryRecord, error) {
	anchorTagsJSON, err := marshalJSONB(memory.AnchorTags, []string{})
	if err != nil {
		return memoryRecord{}, fmt.Errorf("marshal memory anchor tags: %w", err)
	}
	tagScoresJSON, err := marshalJSONB(memory.TagScores, []TagRelevance{})
	if err != nil {
		return memoryRecord{}, fmt.Errorf("marshal memory tag scores: %w", err)
	}
	sourceIssueIDsJSON, err := marshalJSONB(memory.SourceIssueIDs, []string{})
	if err != nil {
		return memoryRecord{}, fmt.Errorf("marshal memory source issue ids: %w", err)
	}
	embeddingVector, err := formatVectorLiteral(memory.Embedding)
	if err != nil {
		return memoryRecord{}, fmt.Errorf("format memory embedding vector: %w", err)
	}

	createdAt := memory.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := memory.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	var lastReinforcedNS int64
	if memory.LastReinforcedAt != nil && !memory.LastReinforcedAt.IsZero() {
		lastReinforcedNS = memory.LastReinforcedAt.UTC().UnixNano()
	}

	return memoryRecord{
		id:                 strings.TrimSpace(memory.ID),
		title:              strings.TrimSpace(memory.Title),
		body:               strings.TrimSpace(memory.Body),
		kind:               string(domain.NormalizeMemoryKind(memory.Kind)),
		anchorTagsJSON:     anchorTagsJSON,
		anchorRegion:       strings.TrimSpace(memory.AnchorRegion),
		tagScoresJSON:      tagScoresJSON,
		embeddingVector:    embeddingVector,
		status:             string(domain.NormalizeMemoryStatus(memory.Status)),
		supersededBy:       strings.TrimSpace(memory.SupersededBy),
		source:             string(domain.NormalizeMemorySource(memory.Source)),
		sourceIssueIDsJSON: sourceIssueIDsJSON,
		confidence:         memory.Confidence,
		createdBy:          strings.TrimSpace(memory.CreatedBy),
		createdAtNS:        createdAt.UTC().UnixNano(),
		updatedAtNS:        updatedAt.UTC().UnixNano(),
		lastReinforcedAtNS: lastReinforcedNS,
		reinforcementCount: int64(memory.ReinforcementCount),
	}, nil
}

func scanMemory(row memoryRow) (domain.Memory, error) {
	memory, _, err := scanMemoryRow(row, false)
	return memory, err
}

func scanMemoryWithSimilarity(row memoryRow) (domain.Memory, float64, error) {
	return scanMemoryRow(row, true)
}

func scanMemoryRow(row memoryRow, withSimilarity bool) (domain.Memory, float64, error) {
	var (
		id, title, body, kind, anchorRegion, status, supersededBy, source, createdBy string
		anchorTagsJSON, tagScoresJSON, sourceIssueIDsJSON                            []byte
		embeddingText                                                                string
		confidence                                                                   float64
		createdAtNS, updatedAtNS, lastReinforcedAtNS, reinforcementCount             int64
		similarity                                                                   float64
	)
	dest := []any{
		&id, &title, &body, &kind, &anchorTagsJSON, &anchorRegion, &tagScoresJSON,
		&embeddingText, &status, &supersededBy, &source, &sourceIssueIDsJSON,
		&confidence, &createdBy, &createdAtNS, &updatedAtNS,
		&lastReinforcedAtNS, &reinforcementCount,
	}
	if withSimilarity {
		dest = append(dest, &similarity)
	}
	if err := row.Scan(dest...); err != nil {
		return domain.Memory{}, 0, err
	}

	anchorTags, err := unmarshalJSONB[[]string](anchorTagsJSON)
	if err != nil {
		return domain.Memory{}, 0, fmt.Errorf("decode memory anchor tags for %q: %w", id, err)
	}
	tagScores, err := unmarshalJSONB[[]TagRelevance](tagScoresJSON)
	if err != nil {
		return domain.Memory{}, 0, fmt.Errorf("decode memory tag scores for %q: %w", id, err)
	}
	sourceIssueIDs, err := unmarshalJSONB[[]string](sourceIssueIDsJSON)
	if err != nil {
		return domain.Memory{}, 0, fmt.Errorf("decode memory source issue ids for %q: %w", id, err)
	}
	embedding, err := parseEmbeddingText(embeddingText)
	if err != nil {
		return domain.Memory{}, 0, fmt.Errorf("decode memory embedding for %q: %w", id, err)
	}

	var lastReinforcedAt *time.Time
	if lastReinforcedAtNS > 0 {
		t := time.Unix(0, lastReinforcedAtNS).UTC()
		lastReinforcedAt = &t
	}

	return domain.Memory{
		ID:                 id,
		Title:              title,
		Body:               body,
		Kind:               domain.NormalizeMemoryKind(domain.MemoryKind(kind)),
		AnchorTags:         anchorTags,
		AnchorRegion:       anchorRegion,
		TagScores:          tagScores,
		Status:             domain.NormalizeMemoryStatus(domain.MemoryStatus(status)),
		SupersededBy:       supersededBy,
		Source:             domain.NormalizeMemorySource(domain.MemorySource(source)),
		SourceIssueIDs:     sourceIssueIDs,
		Confidence:         confidence,
		CreatedBy:          createdBy,
		CreatedAt:          time.Unix(0, createdAtNS).UTC(),
		UpdatedAt:          time.Unix(0, updatedAtNS).UTC(),
		LastReinforcedAt:   lastReinforcedAt,
		ReinforcementCount: int(reinforcementCount),
		Embedding:          embedding,
	}, similarity, nil
}
