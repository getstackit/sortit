package issues

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"splat/internal/issues/issuesdb"
)

func (s *PostgresStore) SaveIssue(ctx context.Context, issue Issue) error {
	record, err := recordFromIssue(issue)
	if err != nil {
		return err
	}

	if err := s.queries.InsertIssue(ctx, issuesdb.InsertIssueParams{
		ID:                record.ID,
		Raw:               record.Raw,
		TagsJson:          record.TagsJSON,
		CreatedBy:         record.CreatedBy,
		CreatedAtUnixNano: record.CreatedAtUnixNano,
		Status:            record.Status,
		ClosedAtUnixNano:  record.ClosedAtUnixNano,
		ClosedBy:          record.ClosedBy,
		TagScoresJson:     record.TagScoresJSON,
		EmbeddingJson:     record.EmbeddingJSON,
		AssignedTo:        record.AssignedTo,
	}); err != nil {
		return fmt.Errorf("save issue: %w", err)
	}
	if err := syncIssueEmbeddingVector(ctx, s.db, record.ID, issue.Embedding); err != nil {
		return err
	}

	if err := insertIssuePosts(ctx, s.queries, issue.Discussion); err != nil {
		return err
	}

	return nil
}

func (s *PostgresStore) SaveIssuePost(ctx context.Context, post IssuePost) error {
	if err := s.queries.InsertIssuePost(ctx, issuesdb.InsertIssuePostParams{
		ID:                strings.TrimSpace(post.ID),
		IssueID:           strings.TrimSpace(post.IssueID),
		Raw:               strings.TrimSpace(post.Raw),
		CreatedBy:         strings.TrimSpace(post.CreatedBy),
		CreatedAtUnixNano: post.CreatedAt.UTC().UnixNano(),
		Sequence:          int64(post.Sequence),
		Kind:              post.Kind,
	}); err != nil {
		return fmt.Errorf("save issue post: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateIssueFields(ctx context.Context, id string, fields IssueFieldUpdate) error {
	id = strings.TrimSpace(id)

	if fields.Status != nil && *fields.Status == StatusClosed && fields.ClosedAt != nil && fields.ClosedBy != nil {
		if err := s.queries.CloseIssue(ctx, issuesdb.CloseIssueParams{
			ID:               id,
			ClosedAtUnixNano: fields.ClosedAt.UTC().UnixNano(),
			ClosedBy:         *fields.ClosedBy,
		}); err != nil {
			return fmt.Errorf("close issue fields: %w", err)
		}
	}

	if fields.Status != nil && *fields.Status == StatusOpen {
		if err := s.queries.ReopenIssue(ctx, id); err != nil {
			return fmt.Errorf("reopen issue fields: %w", err)
		}
	}

	if fields.AssignedTo != nil {
		if err := s.queries.AssignIssue(ctx, issuesdb.AssignIssueParams{
			AssignedTo: *fields.AssignedTo,
			ID:         id,
		}); err != nil {
			return fmt.Errorf("assign issue fields: %w", err)
		}
	}

	if fields.Raw != nil {
		record, err := buildRefinementRecord(id, fields)
		if err != nil {
			return err
		}
		if err := s.queries.UpdateIssueRefinement(ctx, record); err != nil {
			return fmt.Errorf("update issue refinement fields: %w", err)
		}
		if err := syncIssueEmbeddingVector(ctx, s.db, id, fields.Embedding); err != nil {
			return err
		}
	}

	return nil
}

func buildRefinementRecord(id string, fields IssueFieldUpdate) (issuesdb.UpdateIssueRefinementParams, error) {
	tags := fields.Tags
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, err := marshalJSONB(tags, []string{})
	if err != nil {
		return issuesdb.UpdateIssueRefinementParams{}, fmt.Errorf("marshal tags: %w", err)
	}
	tagScores := fields.TagScores
	if tagScores == nil {
		tagScores = []TagRelevance{}
	}
	tagScoresJSON, err := marshalJSONB(tagScores, []TagRelevance{})
	if err != nil {
		return issuesdb.UpdateIssueRefinementParams{}, fmt.Errorf("marshal tag scores: %w", err)
	}
	embedding := fields.Embedding
	if embedding == nil {
		embedding = []float64{}
	}
	embeddingJSON, err := marshalJSONB(embedding, []float64{})
	if err != nil {
		return issuesdb.UpdateIssueRefinementParams{}, fmt.Errorf("marshal embedding: %w", err)
	}

	return issuesdb.UpdateIssueRefinementParams{
		Raw:           *fields.Raw,
		TagsJson:      tagsJSON,
		TagScoresJson: tagScoresJSON,
		EmbeddingJson: embeddingJSON,
		ID:            id,
	}, nil
}

func syncIssueEmbeddingVector(ctx context.Context, db issuesdb.DBTX, issueID string, embedding []float64) error {
	vectorLiteral, err := formatVectorLiteral(embedding)
	if err != nil {
		return fmt.Errorf("format embedding vector for issue %q: %w", issueID, err)
	}
	if _, err := db.ExecContext(
		ctx,
		`UPDATE issues SET embedding_vector = $1::vector WHERE id = $2`,
		vectorLiteral,
		issueID,
	); err != nil {
		return fmt.Errorf("sync embedding vector for issue %q: %w", issueID, err)
	}
	return nil
}

func formatVectorLiteral(values []float64) (any, error) {
	if len(values) == 0 {
		return nil, nil
	}

	var builder strings.Builder
	builder.Grow(len(values) * 12)
	builder.WriteByte('[')
	for i, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("non-finite value at index %d", i)
		}
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(value, 'g', -1, 64))
	}
	builder.WriteByte(']')
	return builder.String(), nil
}

func (s *PostgresStore) SaveOperation(ctx context.Context, op IssueOperation) error {
	if err := insertIssueOperation(ctx, s.queries, op); err != nil {
		return err
	}
	for i, p := range op.Participants {
		if err := insertIssueOperationParticipant(ctx, s.queries, op.ID, p, i+1); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) SaveLink(ctx context.Context, link IssueLink) error {
	return insertIssueLink(ctx, s.queries, link)
}
