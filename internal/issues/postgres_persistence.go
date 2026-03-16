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
	if err := updateIssueEnrichmentState(ctx, s.db, record.ID, issueFieldUpdateForIssue(issue)); err != nil {
		return err
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
			ClosedReason:     derefString(fields.ClosedReason),
			ClosedReasonNote: derefString(fields.ClosedReasonNote),
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
	if err := updateIssueEnrichmentState(ctx, s.db, id, fields); err != nil {
		return err
	}

	return nil
}

func issueFieldUpdateForIssue(issue Issue) IssueFieldUpdate {
	status := normalizeIssueEnrichmentStatus(issue.EnrichmentStatus)
	errText := strings.TrimSpace(issue.EnrichmentError)
	target := issue.EnrichmentTargetSequence
	return IssueFieldUpdate{
		EnrichmentStatus:         &status,
		EnrichmentError:          &errText,
		EnrichmentTargetSequence: &target,
	}
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

func updateIssueEnrichmentState(ctx context.Context, db issuesdb.DBTX, issueID string, fields IssueFieldUpdate) error {
	if fields.EnrichmentStatus == nil && fields.EnrichmentError == nil && fields.EnrichmentTargetSequence == nil {
		return nil
	}

	var (
		statusParam any
		errorParam  any
		targetParam any
	)
	if fields.EnrichmentStatus != nil {
		status := string(normalizeIssueEnrichmentStatus(*fields.EnrichmentStatus))
		statusParam = status
	}
	if fields.EnrichmentError != nil {
		errorParam = strings.TrimSpace(*fields.EnrichmentError)
	}
	if fields.EnrichmentTargetSequence != nil {
		targetParam = int64(*fields.EnrichmentTargetSequence)
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE issues
		 SET enrichment_status = COALESCE($1::text, enrichment_status),
		     enrichment_error = COALESCE($2::text, enrichment_error),
		     enrichment_target_sequence = COALESCE($3::bigint, enrichment_target_sequence)
		 WHERE id = $4`,
		statusParam,
		errorParam,
		targetParam,
		issueID,
	); err != nil {
		return fmt.Errorf("update issue enrichment state for %q: %w", issueID, err)
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

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *PostgresStore) MergeTags(ctx context.Context, canonical string, aliases []string) error {
	canonical = sanitizeTagName(canonical)
	if canonical == "" {
		return fmt.Errorf("canonical tag name is required")
	}

	aliasSet := make(map[string]struct{}, len(aliases))
	normalizedAliases := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		norm := sanitizeTagName(alias)
		if norm == "" || norm == canonical {
			continue
		}
		if _, ok := aliasSet[norm]; ok {
			continue
		}
		aliasSet[norm] = struct{}{}
		normalizedAliases = append(normalizedAliases, norm)
	}
	if len(normalizedAliases) == 0 {
		return nil
	}

	// Load all issues that reference any alias tag
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tags_json, tag_scores_json FROM issues`)
	if err != nil {
		return fmt.Errorf("list issues for tag merge: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	type issueUpdate struct {
		id        string
		tags      []string
		tagScores []TagRelevance
	}
	var updates []issueUpdate

	for rows.Next() {
		var (
			id            string
			tagsJSON      []byte
			tagScoresJSON []byte
		)
		if err := rows.Scan(&id, &tagsJSON, &tagScoresJSON); err != nil {
			return fmt.Errorf("scan issue for tag merge: %w", err)
		}

		tags, err := unmarshalJSONB[[]string](tagsJSON)
		if err != nil {
			return fmt.Errorf("decode tags for issue %q: %w", id, err)
		}
		tagScores, err := unmarshalJSONB[[]TagRelevance](tagScoresJSON)
		if err != nil {
			return fmt.Errorf("decode tag scores for issue %q: %w", id, err)
		}

		newTags := mergeTagList(tags, canonical, aliasSet)
		newScores := mergeTagScores(tagScores, canonical, aliasSet)

		if len(newTags) != len(tags) || len(newScores) != len(tagScores) {
			updates = append(updates, issueUpdate{id: id, tags: newTags, tagScores: newScores})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate issues for tag merge: %w", err)
	}

	for _, update := range updates {
		tagsJSON, err := marshalJSONB(update.tags, []string{})
		if err != nil {
			return fmt.Errorf("marshal merged tags for %q: %w", update.id, err)
		}
		tagScoresJSON, err := marshalJSONB(update.tagScores, []TagRelevance{})
		if err != nil {
			return fmt.Errorf("marshal merged tag scores for %q: %w", update.id, err)
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE issues SET tags_json = $1, tag_scores_json = $2 WHERE id = $3`,
			tagsJSON, tagScoresJSON, update.id,
		); err != nil {
			return fmt.Errorf("update merged tags for issue %q: %w", update.id, err)
		}
	}

	// Delete alias tags from the tags table
	for _, alias := range normalizedAliases {
		if _, err := s.db.ExecContext(ctx,
			`DELETE FROM tags WHERE LOWER(name) = $1`, alias,
		); err != nil {
			return fmt.Errorf("delete alias tag %q: %w", alias, err)
		}
	}

	// Record merge history
	for _, alias := range normalizedAliases {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO tag_merge_history (canonical_name, alias_name) VALUES ($1, $2)`,
			canonical, alias,
		); err != nil {
			return fmt.Errorf("record tag merge history: %w", err)
		}
	}

	return nil
}

func (s *PostgresStore) DismissTagMerge(ctx context.Context, canonical string, alias string) error {
	canonical = sanitizeTagName(canonical)
	alias = sanitizeTagName(alias)
	if canonical == "" || alias == "" {
		return fmt.Errorf("canonical and alias tag names are required")
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO dismissed_tag_merges (canonical_name, alias_name)
		 VALUES ($1, $2)
		 ON CONFLICT (canonical_name, alias_name) DO NOTHING`,
		canonical, alias,
	); err != nil {
		return fmt.Errorf("dismiss tag merge: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListDismissedTagMerges(ctx context.Context) ([]DismissedTagMerge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT canonical_name, alias_name FROM dismissed_tag_merges ORDER BY dismissed_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list dismissed tag merges: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []DismissedTagMerge
	for rows.Next() {
		var d DismissedTagMerge
		if err := rows.Scan(&d.CanonicalName, &d.AliasName); err != nil {
			return nil, fmt.Errorf("scan dismissed tag merge: %w", err)
		}
		items = append(items, d)
	}
	return items, rows.Err()
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
