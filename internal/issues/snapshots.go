package issues

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues/issuesdb"
)

func issueSnapshotFromFieldUpdate(id string, fields IssueFieldUpdate) (IssueSnapshot, bool) {
	if fields.Raw == nil || fields.EnrichmentTargetSequence == nil {
		return IssueSnapshot{}, false
	}

	return IssueSnapshot{
		IssueID:   strings.TrimSpace(id),
		Sequence:  *fields.EnrichmentTargetSequence,
		Raw:       strings.TrimSpace(*fields.Raw),
		Tags:      append([]string(nil), fields.Tags...),
		TagScores: copyTagScores(fields.TagScores),
		CreatedAt: time.Now().UTC(),
		Embedding: copyEmbedding(fields.Embedding),
	}, true
}

func saveIssueSnapshot(ctx context.Context, db issuesdb.DBTX, snapshot IssueSnapshot) error {
	tagsJSON, err := marshalJSONB(snapshot.Tags, []string{})
	if err != nil {
		return fmt.Errorf("marshal snapshot tags: %w", err)
	}
	tagScoresJSON, err := marshalJSONB(snapshot.TagScores, []domain.TagRelevance{})
	if err != nil {
		return fmt.Errorf("marshal snapshot tag scores: %w", err)
	}
	embeddingVector, err := formatVectorLiteral(snapshot.Embedding)
	if err != nil {
		return fmt.Errorf("format snapshot embedding: %w", err)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO issue_snapshots (
		    issue_id, sequence, raw, tags_json, tag_scores_json, embedding_vector, created_at_unix_nano
		) VALUES ($1, $2, $3, $4, $5, $6::vector, $7)
		ON CONFLICT (issue_id, sequence) DO NOTHING`,
		strings.TrimSpace(snapshot.IssueID),
		snapshot.Sequence,
		strings.TrimSpace(snapshot.Raw),
		tagsJSON,
		tagScoresJSON,
		embeddingVector,
		snapshot.CreatedAt.UTC().UnixNano(),
	); err != nil {
		return fmt.Errorf("save issue snapshot %q/%d: %w", snapshot.IssueID, snapshot.Sequence, err)
	}
	return nil
}

func listIssueSnapshots(ctx context.Context, db issuesdb.DBTX, issueID string) ([]IssueSnapshot, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT issue_id, sequence, raw, tags_json, tag_scores_json,
		        COALESCE(embedding_vector::text, ''), created_at_unix_nano
		 FROM issue_snapshots
		 WHERE issue_id = $1
		 ORDER BY sequence ASC`,
		strings.TrimSpace(issueID),
	)
	if err != nil {
		return nil, fmt.Errorf("list issue snapshots: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	snapshots := make([]IssueSnapshot, 0)
	for rows.Next() {
		var (
			snapshot      IssueSnapshot
			tagsJSON      []byte
			tagScoresJSON []byte
			embeddingText string
			createdAtNano int64
		)
		if err := rows.Scan(
			&snapshot.IssueID,
			&snapshot.Sequence,
			&snapshot.Raw,
			&tagsJSON,
			&tagScoresJSON,
			&embeddingText,
			&createdAtNano,
		); err != nil {
			return nil, fmt.Errorf("scan issue snapshot: %w", err)
		}
		tags, err := unmarshalJSONB[[]string](tagsJSON)
		if err != nil {
			return nil, fmt.Errorf("decode snapshot tags: %w", err)
		}
		tagScores, err := unmarshalJSONB[[]domain.TagRelevance](tagScoresJSON)
		if err != nil {
			return nil, fmt.Errorf("decode snapshot tag scores: %w", err)
		}
		embedding, err := parseEmbeddingText(embeddingText)
		if err != nil {
			return nil, fmt.Errorf("decode snapshot embedding: %w", err)
		}
		snapshot.Tags = tags
		snapshot.TagScores = tagScores
		snapshot.Embedding = embedding
		snapshot.CreatedAt = time.Unix(0, createdAtNano).UTC()
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue snapshots: %w", err)
	}
	return snapshots, nil
}
