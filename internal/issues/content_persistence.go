package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sortit/internal/issues/issuesdb"
)

type issueContentState struct {
	IssueID     string
	Raw         string
	Tags        []string
	TagScores   []TagRelevance
	Embedding   []float64
	LastEventID string
	EventCount  int64
	UpdatedAt   time.Time
}

type issueContentFactRecord struct {
	ID        string
	IssueID   string
	Sequence  int64
	Kind      string
	CreatedBy string
	CreatedAt time.Time
	Payload   json.RawMessage
	Source    string
	SourceID  string
	Inferred  bool
}

type issueContentPayload struct {
	Raw       string         `json:"raw"`
	Tags      []string       `json:"tags"`
	TagScores []TagRelevance `json:"tagScores"`
	Embedding []float64      `json:"embedding,omitempty"`
}

func syncIssueContentOnCreate(ctx context.Context, db issuesdb.DBTX, issue Issue) error {
	state := issueContentState{
		IssueID:   strings.TrimSpace(issue.ID),
		Raw:       strings.TrimSpace(issue.Raw),
		Tags:      append([]string(nil), issue.Tags...),
		TagScores: copyTagScores(issue.TagScores),
		Embedding: copyEmbedding(issue.Embedding),
	}

	fact := issueContentFactRecord{
		ID:        NewIssueContentFactID(),
		IssueID:   state.IssueID,
		Sequence:  1,
		Kind:      "created",
		CreatedBy: DefaultActor(issue.CreatedBy),
		CreatedAt: issue.CreatedAt.UTC(),
		Payload:   mustIssueContentJSON(state),
		Source:    "issue_create",
		SourceID:  state.IssueID + ":content",
	}
	if err := insertIssueContentFact(ctx, db, fact); err != nil {
		return err
	}

	state.LastEventID = fact.ID
	state.EventCount = 1
	state.UpdatedAt = fact.CreatedAt
	return upsertIssueContentProjection(ctx, db, state)
}

func applyIssueContentFieldUpdate(ctx context.Context, db issuesdb.DBTX, issueID string, fields IssueFieldUpdate) error {
	if fields.Raw == nil {
		return nil
	}

	state, err := loadIssueContentState(ctx, db, issueID)
	if err != nil {
		return err
	}
	state.Raw = strings.TrimSpace(*fields.Raw)
	if fields.Tags != nil {
		state.Tags = append([]string(nil), fields.Tags...)
	}
	if fields.TagScores != nil {
		state.TagScores = copyTagScores(fields.TagScores)
	}
	if fields.Embedding != nil {
		state.Embedding = copyEmbedding(fields.Embedding)
	}

	now := time.Now().UTC()
	fact := issueContentFactRecord{
		ID:        NewIssueContentFactID(),
		IssueID:   strings.TrimSpace(issueID),
		Sequence:  state.EventCount + 1,
		Kind:      "content_updated",
		CreatedBy: "system",
		CreatedAt: now,
		Payload:   mustIssueContentJSON(state),
		Source:    "issue_update",
		SourceID:  NewIssueContentFactID(),
	}
	if err := insertIssueContentFact(ctx, db, fact); err != nil {
		return err
	}

	state.LastEventID = fact.ID
	state.EventCount++
	state.UpdatedAt = now
	return upsertIssueContentProjection(ctx, db, state)
}

func applyIssueContentTagMerge(
	ctx context.Context,
	db issuesdb.DBTX,
	issueID string,
	tags []string,
	tagScores []TagRelevance,
	createdAt time.Time,
) error {
	state, err := loadIssueContentState(ctx, db, issueID)
	if err != nil {
		return err
	}
	state.Tags = append([]string(nil), tags...)
	state.TagScores = copyTagScores(tagScores)

	fact := issueContentFactRecord{
		ID:        NewIssueContentFactID(),
		IssueID:   strings.TrimSpace(issueID),
		Sequence:  state.EventCount + 1,
		Kind:      "tags_merged",
		CreatedBy: "system",
		CreatedAt: createdAt.UTC(),
		Payload:   mustIssueContentJSON(state),
		Source:    "tag_merge",
		SourceID:  NewIssueContentFactID(),
	}
	if err := insertIssueContentFact(ctx, db, fact); err != nil {
		return err
	}

	state.LastEventID = fact.ID
	state.EventCount++
	state.UpdatedAt = createdAt.UTC()
	if err := upsertIssueContentProjection(ctx, db, state); err != nil {
		return err
	}
	return syncIssueSearchProjection(ctx, db, state.IssueID)
}

func loadIssueContentState(ctx context.Context, db issuesdb.DBTX, issueID string) (issueContentState, error) {
	var (
		state         issueContentState
		tagsJSON      json.RawMessage
		tagScoresJSON json.RawMessage
		embeddingText string
		updatedAtNano int64
	)
	if err := db.QueryRowContext(
		ctx,
		`SELECT i.id,
		        COALESCE(c.raw, i.raw) AS raw,
		        COALESCE(c.tags_json, i.tags_json) AS tags_json,
		        COALESCE(c.tag_scores_json, i.tag_scores_json) AS tag_scores_json,
		        COALESCE(c.embedding_vector::text, i.embedding_vector::text, '') AS embedding_text,
		        COALESCE(c.last_event_id, '') AS last_event_id,
		        COALESCE(c.event_count, 0) AS event_count,
		        COALESCE(c.updated_at_unix_nano, i.created_at_unix_nano) AS updated_at_unix_nano
		 FROM issues i
		 LEFT JOIN issue_content_projections c ON c.issue_id = i.id
		 WHERE i.id = $1`,
		strings.TrimSpace(issueID),
	).Scan(
		&state.IssueID,
		&state.Raw,
		&tagsJSON,
		&tagScoresJSON,
		&embeddingText,
		&state.LastEventID,
		&state.EventCount,
		&updatedAtNano,
	); err != nil {
		return issueContentState{}, fmt.Errorf("load issue content state for %q: %w", issueID, err)
	}

	tags, err := unmarshalJSONB[[]string](tagsJSON)
	if err != nil {
		return issueContentState{}, fmt.Errorf("decode issue content tags for %q: %w", issueID, err)
	}
	tagScores, err := unmarshalJSONB[[]TagRelevance](tagScoresJSON)
	if err != nil {
		return issueContentState{}, fmt.Errorf("decode issue content tag scores for %q: %w", issueID, err)
	}
	embedding, err := parseEmbeddingText(embeddingText)
	if err != nil {
		return issueContentState{}, fmt.Errorf("decode issue content embedding for %q: %w", issueID, err)
	}

	state.Raw = strings.TrimSpace(state.Raw)
	state.Tags = tags
	state.TagScores = tagScores
	state.Embedding = embedding
	state.UpdatedAt = time.Unix(0, updatedAtNano).UTC()
	return state, nil
}

func insertIssueContentFact(ctx context.Context, db issuesdb.DBTX, fact issueContentFactRecord) error {
	if _, err := db.ExecContext(
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
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)`,
		strings.TrimSpace(fact.ID),
		strings.TrimSpace(fact.IssueID),
		fact.Sequence,
		strings.TrimSpace(fact.Kind),
		strings.TrimSpace(fact.CreatedBy),
		fact.CreatedAt.UTC().UnixNano(),
		[]byte(fact.Payload),
		strings.TrimSpace(fact.Source),
		strings.TrimSpace(fact.SourceID),
		fact.Inferred,
	); err != nil {
		return fmt.Errorf("insert issue content fact %q: %w", fact.ID, err)
	}
	return nil
}

func upsertIssueContentProjection(ctx context.Context, db issuesdb.DBTX, state issueContentState) error {
	tagsJSON, err := marshalJSONB(state.Tags, []string{})
	if err != nil {
		return fmt.Errorf("marshal issue content projection tags: %w", err)
	}
	tagScoresJSON, err := marshalJSONB(state.TagScores, []TagRelevance{})
	if err != nil {
		return fmt.Errorf("marshal issue content projection tag scores: %w", err)
	}
	embeddingVector, err := formatVectorLiteral(state.Embedding)
	if err != nil {
		return fmt.Errorf("format issue content projection embedding: %w", err)
	}

	if _, err := db.ExecContext(
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
		 ) VALUES ($1, $2, $3, $4, $5::vector, $6, $7, $8)
		 ON CONFLICT (issue_id) DO UPDATE
		 SET raw = EXCLUDED.raw,
		     tags_json = EXCLUDED.tags_json,
		     tag_scores_json = EXCLUDED.tag_scores_json,
		     embedding_vector = EXCLUDED.embedding_vector,
		     last_event_id = EXCLUDED.last_event_id,
		     event_count = EXCLUDED.event_count,
		     updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		strings.TrimSpace(state.IssueID),
		state.Raw,
		tagsJSON,
		tagScoresJSON,
		embeddingVector,
		strings.TrimSpace(state.LastEventID),
		state.EventCount,
		state.UpdatedAt.UTC().UnixNano(),
	); err != nil {
		return fmt.Errorf("upsert issue content projection for %q: %w", state.IssueID, err)
	}
	return nil
}

type issueSearchProjection struct {
	Title string
	Body  string
	Tags  string
}

func syncIssueSearchProjection(ctx context.Context, db issuesdb.DBTX, issueID string) error {
	state, err := loadIssueContentState(ctx, db, issueID)
	if err != nil {
		return err
	}

	projection, err := buildIssueSearchProjection(ctx, db, state)
	if err != nil {
		return err
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE issue_content_projections
		 SET search_title = $2,
		     search_body = $3,
		     search_tags = $4
		 WHERE issue_id = $1`,
		strings.TrimSpace(state.IssueID),
		projection.Title,
		projection.Body,
		projection.Tags,
	); err != nil {
		return fmt.Errorf("update issue search projection for %q: %w", state.IssueID, err)
	}

	return nil
}

func buildIssueSearchProjection(ctx context.Context, db issuesdb.DBTX, state issueContentState) (issueSearchProjection, error) {
	title := strings.TrimSpace(state.Raw)

	rows, err := db.QueryContext(
		ctx,
		`SELECT raw
		 FROM issue_posts
		 WHERE issue_id = $1
		 ORDER BY sequence ASC, id ASC`,
		strings.TrimSpace(state.IssueID),
	)
	if err != nil {
		return issueSearchProjection{}, fmt.Errorf("list issue posts for %q search projection: %w", state.IssueID, err)
	}
	defer rows.Close() //nolint:errcheck

	bodyParts := make([]string, 0, max(1, int(state.EventCount)))
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return issueSearchProjection{}, fmt.Errorf("scan issue post for %q search projection: %w", state.IssueID, err)
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		bodyParts = append(bodyParts, raw)
	}
	if err := rows.Err(); err != nil {
		return issueSearchProjection{}, fmt.Errorf("iterate issue posts for %q search projection: %w", state.IssueID, err)
	}

	body := strings.Join(bodyParts, "\n\n")
	if body == "" {
		body = title
	}

	tagParts := make([]string, 0, len(state.Tags)+len(state.TagScores))
	seen := make(map[string]struct{}, len(state.Tags)+len(state.TagScores))
	appendTag := func(value string) {
		tag := sanitizeTagName(value)
		if tag == "" {
			return
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		tagParts = append(tagParts, tag)
	}
	for _, tag := range state.Tags {
		appendTag(tag)
	}
	for _, score := range state.TagScores {
		appendTag(score.Tag)
	}

	return issueSearchProjection{
		Title: title,
		Body:  body,
		Tags:  strings.Join(tagParts, " "),
	}, nil
}

func mustIssueContentJSON(state issueContentState) json.RawMessage {
	encoded, err := json.Marshal(issueContentPayload{
		Raw:       state.Raw,
		Tags:      append([]string(nil), state.Tags...),
		TagScores: copyTagScores(state.TagScores),
		Embedding: copyEmbedding(state.Embedding),
	})
	if err != nil {
		panic(fmt.Sprintf("marshal issue content payload: %v", err))
	}
	return json.RawMessage(encoded)
}
