package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

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
		Column10:          record.EmbeddingVector,
		AssignedTo:        record.AssignedTo,
	}); err != nil {
		return fmt.Errorf("save issue: %w", err)
	}
	if err := syncIssueLifecycleOnCreate(ctx, s.db, issue); err != nil {
		return err
	}
	if err := syncIssueContentOnCreate(ctx, s.db, issue); err != nil {
		return err
	}
	if err := updateIssueEnrichmentState(ctx, s.db, record.ID, issueFieldUpdateForIssue(issue)); err != nil {
		return err
	}

	if err := insertIssuePosts(ctx, s.queries, issue.Discussion); err != nil {
		return err
	}
	if err := syncIssueSearchProjection(ctx, s.db, record.ID); err != nil {
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
	if err := syncIssueSearchProjection(ctx, s.db, strings.TrimSpace(post.IssueID)); err != nil {
		return err
	}
	return nil
}

func (s *PostgresStore) UpdateIssueFields(ctx context.Context, id string, fields IssueFieldUpdate) error {
	id = strings.TrimSpace(id)

	if err := applyLifecycleIssueFieldUpdate(ctx, s.db, id, fields); err != nil {
		return err
	}

	if fields.Raw != nil {
		if err := applyIssueContentFieldUpdate(ctx, s.db, id, fields); err != nil {
			return err
		}
		if err := syncIssueSearchProjection(ctx, s.db, id); err != nil {
			return err
		}
		if snapshot, ok := issueSnapshotFromFieldUpdate(id, fields); ok {
			if err := saveIssueSnapshot(ctx, s.db, snapshot); err != nil {
				return err
			}
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

// parseEmbeddingText parses the pgvector ::text output format "[0.1,0.2,...]"
// back into a float64 slice. Returns nil for empty strings.
func parseEmbeddingText(text string) ([]float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	var result []float64
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parse embedding text: %w", err)
	}
	return result, nil
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin merge tags tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Load all issues that reference any alias tag
	rows, err := tx.QueryContext(ctx,
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

		if !slices.Equal(newTags, tags) || !equalTagScores(newScores, tagScores) {
			updates = append(updates, issueUpdate{id: id, tags: newTags, tagScores: newScores})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate issues for tag merge: %w", err)
	}

	mergedAt := time.Now().UTC()
	for _, update := range updates {
		if err := applyIssueContentTagMerge(ctx, tx, update.id, update.tags, update.tagScores, mergedAt); err != nil {
			return fmt.Errorf("update merged content for issue %q: %w", update.id, err)
		}
	}

	if _, err := ensureActiveTagProjection(ctx, tx, canonical, "", mergedAt, "tag_merge", "canonical:"+canonical); err != nil {
		return err
	}

	for _, alias := range normalizedAliases {
		sourceID := canonical + "->" + alias + ":" + mergedAt.Format(time.RFC3339Nano)
		if err := appendTagMerge(ctx, tx, canonical, alias, "", mergedAt, "tag_merge", sourceID); err != nil {
			return err
		}
	}

	// Record merge history
	for _, alias := range normalizedAliases {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tag_merge_history (canonical_name, alias_name) VALUES ($1, $2)`,
			canonical, alias,
		); err != nil {
			return fmt.Errorf("record tag merge history: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit merge tags: %w", err)
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
