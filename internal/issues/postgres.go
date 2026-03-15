package issues

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"splat/internal/issues/issuesdb"
)

const (
	pgDriverName          = "pgx"
	schemaMigrationsTable = "schema_migrations"
)

//go:embed pgmigrations/*.sql
var pgMigrationsFS embed.FS

type PostgresStore struct {
	db      *sql.DB
	queries *issuesdb.Queries
	logger  *slog.Logger
}

func OpenPostgresStore(ctx context.Context, connString string) (*PostgresStore, error) {
	connString = strings.TrimSpace(connString)
	if connString == "" {
		return nil, fmt.Errorf("postgres connection string is required")
	}

	db, err := sql.Open(pgDriverName, connString)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	store := &PostgresStore{
		db:      db,
		queries: issuesdb.New(db),
		logger:  slog.Default().With("component", "issues_store"),
	}

	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) DB() *sql.DB {
	return s.db
}

func (s *PostgresStore) SetLogger(logger *slog.Logger) {
	if logger == nil {
		s.logger = slog.Default().With("component", "issues_store")
		return
	}
	s.logger = logger.With("component", "issues_store")
}

func (s *PostgresStore) List(ctx context.Context) ([]Issue, error) {
	rows, err := s.queries.ListIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}

	items := make([]Issue, 0, len(rows))
	for _, row := range rows {
		issue, err := issueFromQuery(issueModelFromListIssuesRow(row))
		if err != nil {
			return nil, err
		}
		items = append(items, issue)
	}
	states, err := loadIssueEnrichmentStates(ctx, s.db, s.logger, issueIDs(items))
	if err != nil {
		return nil, err
	}
	items = applyIssueEnrichmentStates(items, states)
	return cloneIssues(items), nil
}

func (s *PostgresStore) ListIssueMetadata(ctx context.Context) ([]Issue, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, raw, status, assigned_to
		 FROM issues
		 ORDER BY created_at_unix_nano DESC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list issue metadata: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	items := make([]Issue, 0)
	for rows.Next() {
		var (
			id         string
			raw        string
			status     string
			assignedTo string
		)
		if err := rows.Scan(&id, &raw, &status, &assignedTo); err != nil {
			return nil, fmt.Errorf("scan issue metadata: %w", err)
		}
		items = append(items, Issue{
			ID:         id,
			Raw:        raw,
			Status:     normalizeIssueStatus(IssueStatus(status)),
			AssignedTo: strings.TrimSpace(assignedTo),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue metadata: %w", err)
	}

	return cloneIssues(items), nil
}

func (s *PostgresStore) ListIssueEmbeddingSimilarities(
	ctx context.Context,
	query []float64,
	limit int,
) ([]IssueEmbeddingSimilarity, int, float64, error) {
	if len(query) == 0 {
		return []IssueEmbeddingSimilarity{}, 0, 0, nil
	}
	if limit <= 0 {
		limit = 8
	}

	vectorLiteral, err := formatVectorLiteral(query)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("format query embedding: %w", err)
	}

	var (
		compared int
		average  float64
	)
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*),
		        COALESCE(AVG(1 - (embedding_vector <=> $1::vector)), 0)
		 FROM issues
		 WHERE embedding_vector IS NOT NULL
		   AND vector_dims(embedding_vector) = $2`,
		vectorLiteral,
		len(query),
	).Scan(&compared, &average); err != nil {
		return nil, 0, 0, fmt.Errorf("load issue embedding similarity stats: %w", err)
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, raw, tags_json, (1 - (embedding_vector <=> $1::vector)) AS similarity
		 FROM issues
		 WHERE embedding_vector IS NOT NULL
		   AND vector_dims(embedding_vector) = $2
		 ORDER BY embedding_vector <=> $1::vector ASC, created_at_unix_nano DESC, id ASC
		 LIMIT $3`,
		vectorLiteral,
		len(query),
		limit,
	)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("list issue embedding similarities: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	items := make([]IssueEmbeddingSimilarity, 0, limit)
	for rows.Next() {
		var (
			id       string
			raw      string
			tagsJSON json.RawMessage
			score    float64
		)
		if err := rows.Scan(&id, &raw, &tagsJSON, &score); err != nil {
			return nil, 0, 0, fmt.Errorf("scan issue embedding similarity: %w", err)
		}
		tags, err := unmarshalJSONB[[]string](tagsJSON)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("decode tags for %q: %w", id, err)
		}
		items = append(items, IssueEmbeddingSimilarity{
			ID:         id,
			Raw:        raw,
			Tags:       tags,
			Similarity: math.Round(score*100) / 100,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, fmt.Errorf("iterate issue embedding similarities: %w", err)
	}

	return items, compared, math.Round(average*100) / 100, nil
}

func (s *PostgresStore) ListPeopleAnalytics(ctx context.Context, opts ListOptions) ([]PeopleAnalyticsIssue, error) {
	params := normalizeListIssuesFilteredParams(listIssuesFilteredParams{
		status:     opts.Status,
		assignedTo: opts.AssignedTo,
	})

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT status, assigned_to, tag_scores_json, embedding_json
		 FROM issues
		 WHERE (NOT $1::bool OR status = $2::text)
		   AND (NOT $3::bool OR LOWER(assigned_to) = LOWER($4::text))
		 ORDER BY created_at_unix_nano DESC, id ASC`,
		params.filterStatus,
		params.statusValue,
		params.filterAssignedTo,
		params.assignedToValue,
	)
	if err != nil {
		return nil, fmt.Errorf("list people analytics issues: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	items := make([]PeopleAnalyticsIssue, 0)
	for rows.Next() {
		var (
			status        string
			assignedTo    string
			tagScoresJSON json.RawMessage
			embeddingJSON json.RawMessage
		)
		if err := rows.Scan(&status, &assignedTo, &tagScoresJSON, &embeddingJSON); err != nil {
			return nil, fmt.Errorf("scan people analytics issue: %w", err)
		}
		tagScores, err := unmarshalJSONB[[]TagRelevance](tagScoresJSON)
		if err != nil {
			return nil, fmt.Errorf("decode people analytics tag scores: %w", err)
		}
		embedding, err := unmarshalJSONB[[]float64](embeddingJSON)
		if err != nil {
			return nil, fmt.Errorf("decode people analytics embedding: %w", err)
		}
		items = append(items, PeopleAnalyticsIssue{
			Status:     normalizeIssueStatus(IssueStatus(status)),
			AssignedTo: strings.TrimSpace(assignedTo),
			TagScores:  append([]TagRelevance(nil), tagScores...),
			Embedding:  append([]float64(nil), embedding...),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate people analytics issues: %w", err)
	}

	return items, nil
}

func (s *PostgresStore) ListCompareIssues(ctx context.Context, ids []string) ([]CompareIssue, error) {
	if len(ids) == 0 {
		return []CompareIssue{}, nil
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, embedding_json
		 FROM issues
		 WHERE id = ANY($1)`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("list compare issues: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	byID := make(map[string]CompareIssue, len(ids))
	for rows.Next() {
		var (
			id            string
			embeddingJSON json.RawMessage
		)
		if err := rows.Scan(&id, &embeddingJSON); err != nil {
			return nil, fmt.Errorf("scan compare issue: %w", err)
		}
		embedding, err := unmarshalJSONB[[]float64](embeddingJSON)
		if err != nil {
			return nil, fmt.Errorf("decode compare issue embedding for %q: %w", id, err)
		}
		byID[id] = CompareIssue{
			ID:        id,
			Embedding: append([]float64(nil), embedding...),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate compare issues: %w", err)
	}

	items := make([]CompareIssue, 0, len(ids))
	for _, id := range ids {
		item, ok := byID[id]
		if !ok {
			return nil, ErrNotFound
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *PostgresStore) ListFiltered(ctx context.Context, opts ListOptions) ([]Issue, error) {
	rows, err := s.queries.ListIssuesFiltered(ctx, normalizeListIssuesFilteredParams(listIssuesFilteredParams{
		status:     opts.Status,
		assignedTo: opts.AssignedTo,
		tags:       opts.Tags,
		limit:      opts.Limit,
		offset:     opts.Offset,
	}).sqlc())
	if err != nil {
		return nil, fmt.Errorf("list filtered issues: %w", err)
	}

	items := make([]Issue, 0, len(rows))
	for _, row := range rows {
		issue, err := issueFromQuery(issueModelFromListIssuesFilteredRow(row))
		if err != nil {
			return nil, err
		}
		items = append(items, issue)
	}
	states, err := loadIssueEnrichmentStates(ctx, s.db, s.logger, issueIDs(items))
	if err != nil {
		return nil, err
	}
	items = applyIssueEnrichmentStates(items, states)
	return cloneIssues(items), nil
}

func (s *PostgresStore) SearchIssues(ctx context.Context, opts SemanticSearchOptions) ([]SemanticSearchResult, error) {
	params := normalizeListIssuesFilteredParams(listIssuesFilteredParams{
		status:     opts.Status,
		assignedTo: opts.AssignedTo,
		tags:       opts.Tags,
		excludeID:  opts.ExcludeID,
		limit:      opts.Limit,
		offset:     opts.Offset,
	})

	if strings.EqualFold(strings.TrimSpace(opts.SortBy), "created_at") || len(opts.QueryEmbedding) == 0 {
		rows, err := s.queries.ListIssuesFiltered(ctx, params.sqlc())
		if err != nil {
			return nil, fmt.Errorf("search issues by created_at: %w", err)
		}
		return semanticSearchResultsFromFilteredRows(rows)
	}

	vectorLiteral, err := formatVectorLiteral(opts.QueryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("format query embedding: %w", err)
	}

	rows, err := s.queries.SearchIssuesByEmbedding(ctx, issuesdb.SearchIssuesByEmbeddingParams{
		QueryVector:      vectorLiteral,
		EmbeddingDims:    int32(len(opts.QueryEmbedding)), //nolint:gosec
		FilterStatus:     params.filterStatus,
		Status:           params.statusValue,
		FilterAssignedTo: params.filterAssignedTo,
		AssignedTo:       params.assignedToValue,
		FilterExcludeID:  params.filterExcludeID,
		ExcludeID:        params.excludeIDValue,
		FilterTags:       params.filterTags,
		Tags:             append([]string(nil), params.tagsValue...),
		LimitCount:       int32(params.limit),  //nolint:gosec
		OffsetCount:      int32(params.offset), //nolint:gosec
	})
	if err != nil {
		return nil, fmt.Errorf("search issues by embedding: %w", err)
	}

	results := make([]SemanticSearchResult, 0, len(rows))
	for _, row := range rows {
		issue, err := issueFromQuery(issueModelFromSearchIssuesByEmbeddingRow(row))
		if err != nil {
			return nil, err
		}
		distance, err := float64Value(row.SemanticDistance)
		if err != nil {
			return nil, fmt.Errorf("decode semantic distance for %q: %w", row.ID, err)
		}
		results = append(results, SemanticSearchResult{
			Issue:            issue,
			SemanticDistance: distance,
		})
	}
	items := make([]Issue, 0, len(results))
	for _, result := range results {
		items = append(items, result.Issue)
	}
	states, err := loadIssueEnrichmentStates(ctx, s.db, s.logger, issueIDs(items))
	if err != nil {
		return nil, err
	}
	items = applyIssueEnrichmentStates(items, states)
	for i := range results {
		results[i].Issue = items[i]
	}

	return results, nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Issue, error) {
	return s.GetIssueDetail(ctx, id)
}

func (s *PostgresStore) GetIssueDetail(ctx context.Context, id string) (Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Issue{}, ErrNotFound
	}

	defer s.logServiceTiming(ctx, "PostgresStore.GetIssueDetail", time.Now(), slog.String("issue_id", id))

	issue, err := getIssueWithDiscussion(ctx, loggingIssueQuerier{
		inner:  s.queries,
		logger: s.logger,
	}, id)
	if err != nil {
		return Issue{}, err
	}
	states, err := loadIssueEnrichmentStates(ctx, s.db, s.logger, []string{id})
	if err != nil {
		return Issue{}, err
	}
	return applyIssueEnrichmentStates([]Issue{issue}, states)[0], nil
}

func (s *PostgresStore) GetIssueDetailBase(ctx context.Context, id string) (Issue, error) {
	return getIssueDetailBase(ctx, loggingIssueQuerier{
		inner:  s.queries,
		logger: s.logger,
	}, id)
}

func (s *PostgresStore) ListIssueDetailPosts(ctx context.Context, id string) ([]IssuePost, error) {
	return listIssueDetailPosts(ctx, loggingIssueQuerier{
		inner:  s.queries,
		logger: s.logger,
	}, id)
}

func (s *PostgresStore) ListIssueDetailLinks(ctx context.Context, id string) ([]IssueLink, error) {
	return listIssueDetailLinks(ctx, loggingIssueQuerier{
		inner:  s.queries,
		logger: s.logger,
	}, id)
}

func (s *PostgresStore) ListIssueDetailOperations(ctx context.Context, id string) ([]IssueOperation, error) {
	return listIssueDetailOperations(ctx, loggingIssueQuerier{
		inner:  s.queries,
		logger: s.logger,
	}, id)
}

func (s *PostgresStore) ListIssueDetailReferences(ctx context.Context, ids []string) ([]IssueReference, error) {
	return listIssueDetailReferences(ctx, loggingIssueQuerier{
		inner:  s.queries,
		logger: s.logger,
	}, ids)
}

func (s *PostgresStore) LoadMapProjectionData(ctx context.Context) ([]MapProjectionIssue, []Tag, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, raw, tags_json, status, tag_scores_json, embedding_json, assigned_to
		 FROM issues
		 ORDER BY created_at_unix_nano DESC, id ASC`,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("list issues for map projection: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	items := make([]MapProjectionIssue, 0)
	for rows.Next() {
		var (
			id            string
			raw           string
			tagsJSON      json.RawMessage
			status        string
			tagScoresJSON json.RawMessage
			embeddingJSON json.RawMessage
			assignedTo    string
		)
		if err := rows.Scan(&id, &raw, &tagsJSON, &status, &tagScoresJSON, &embeddingJSON, &assignedTo); err != nil {
			return nil, nil, fmt.Errorf("scan issue for map projection: %w", err)
		}

		tags, err := unmarshalJSONB[[]string](tagsJSON)
		if err != nil {
			return nil, nil, fmt.Errorf("decode tags for %q: %w", id, err)
		}
		tagScores, err := unmarshalJSONB[[]TagRelevance](tagScoresJSON)
		if err != nil {
			return nil, nil, fmt.Errorf("decode tag scores for %q: %w", id, err)
		}
		embedding, err := unmarshalJSONB[[]float64](embeddingJSON)
		if err != nil {
			return nil, nil, fmt.Errorf("decode embedding for %q: %w", id, err)
		}

		items = append(items, MapProjectionIssue{
			ID:         id,
			Raw:        raw,
			Tags:       tags,
			Status:     normalizeIssueStatus(IssueStatus(status)),
			AssignedTo: strings.TrimSpace(assignedTo),
			TagScores:  tagScores,
			Embedding:  embedding,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate issues for map projection: %w", err)
	}

	linkRows, err := s.db.QueryContext(
		ctx,
		`SELECT id, source_issue_id, target_issue_id, type
		 FROM issue_links
		 ORDER BY created_at_unix_nano DESC, id DESC`,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("list issue links for map projection: %w", err)
	}
	defer linkRows.Close() //nolint:errcheck

	linksByIssue := make(map[string][]IssueLink, len(items))
	for linkRows.Next() {
		var (
			id       string
			sourceID string
			targetID string
			linkType string
		)
		if err := linkRows.Scan(&id, &sourceID, &targetID, &linkType); err != nil {
			return nil, nil, fmt.Errorf("scan issue link for map projection: %w", err)
		}

		link := IssueLink{
			ID:            id,
			SourceIssueID: sourceID,
			TargetIssueID: targetID,
			Type:          normalizeIssueLinkType(IssueLinkType(linkType)),
		}
		linksByIssue[sourceID] = append(linksByIssue[sourceID], link)
		if targetID != sourceID {
			linksByIssue[targetID] = append(linksByIssue[targetID], link)
		}
	}
	if err := linkRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate issue links for map projection: %w", err)
	}

	for i := range items {
		items[i].Links = cloneIssueLinks(linksByIssue[items[i].ID])
	}

	tags, err := s.ListTags(ctx)
	if err != nil {
		return nil, nil, err
	}

	return cloneMapProjectionIssues(items), tags, nil
}

func (s *PostgresStore) GetMapProjection(ctx context.Context, revision uint64) ([]byte, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT payload_json FROM map_projections WHERE revision = $1`,
		int64(revision), //nolint:gosec
	)

	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMapProjectionNotFound
		}
		return nil, fmt.Errorf("get map projection: %w", err)
	}
	return payload, nil
}

func (s *PostgresStore) SaveMapProjection(ctx context.Context, revision uint64, payload []byte) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO map_projections (revision, payload_json, created_at_unix_nano)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (revision) DO UPDATE
		 SET payload_json = excluded.payload_json,
		     created_at_unix_nano = excluded.created_at_unix_nano`,
		int64(revision), //nolint:gosec
		payload,
		time.Now().UTC().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("save map projection: %w", err)
	}
	return nil
}

func (s *PostgresStore) InvalidateMapProjections(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM map_projections`); err != nil {
		return fmt.Errorf("invalidate map projections: %w", err)
	}
	return nil
}

func normalizeListOptionTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(tags))
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}

	return normalized
}

type listIssuesFilteredParams struct {
	status           IssueStatus
	assignedTo       string
	tags             []string
	excludeID        string
	limit            int
	offset           int
	filterStatus     bool
	statusValue      string
	filterAssignedTo bool
	assignedToValue  string
	filterTags       bool
	tagsValue        []string
	filterExcludeID  bool
	excludeIDValue   string
}

func (p listIssuesFilteredParams) sqlc() issuesdb.ListIssuesFilteredParams {
	return issuesdb.ListIssuesFilteredParams{
		FilterStatus:     p.filterStatus,
		Status:           p.statusValue,
		FilterAssignedTo: p.filterAssignedTo,
		AssignedTo:       p.assignedToValue,
		FilterExcludeID:  p.filterExcludeID,
		ExcludeID:        p.excludeIDValue,
		FilterTags:       p.filterTags,
		Tags:             append([]string(nil), p.tagsValue...),
		LimitCount:       int32(p.limit),  //nolint:gosec
		OffsetCount:      int32(p.offset), //nolint:gosec
	}
}

func normalizeListIssuesFilteredParams(input listIssuesFilteredParams) listIssuesFilteredParams {
	normalized := input
	normalized.tagsValue = normalizeListOptionTags(input.tags)
	normalized.filterTags = len(normalized.tagsValue) > 0
	normalized.assignedToValue = strings.TrimSpace(input.assignedTo)
	normalized.filterAssignedTo = normalized.assignedToValue != ""
	normalized.excludeIDValue = strings.TrimSpace(input.excludeID)
	normalized.filterExcludeID = normalized.excludeIDValue != ""
	normalized.statusValue, normalized.filterStatus = issueStatusFilterValue(input.status)
	if normalized.limit <= 0 {
		normalized.limit = 2147483647
	}
	if normalized.offset < 0 {
		normalized.offset = 0
	}
	return normalized
}

func issueStatusFilterValue(status IssueStatus) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(StatusOpen):
		return string(StatusOpen), true
	case string(StatusClosed):
		return string(StatusClosed), true
	default:
		return "", false
	}
}

func float64Value(value any) (float64, error) {
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case float32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case []byte:
		return strconv.ParseFloat(string(typed), 64)
	case string:
		return strconv.ParseFloat(typed, 64)
	default:
		return 0, fmt.Errorf("unsupported float value type %T", value)
	}
}

func (s *PostgresStore) Replace(ctx context.Context, next []Issue) error {
	if err := s.ensureDestructiveResetAllowed(ctx); err != nil {
		return err
	}

	items := cloneIssues(next)
	slices.SortStableFunc(items, compareIssueOrder)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace issues tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)
	if err := qtx.DeleteAllEvents(ctx); err != nil {
		return fmt.Errorf("clear events: %w", err)
	}
	if err := qtx.DeleteAllIssueLinks(ctx); err != nil {
		return fmt.Errorf("clear issue links: %w", err)
	}
	if err := qtx.DeleteAllIssueOperationParticipants(ctx); err != nil {
		return fmt.Errorf("clear issue operation participants: %w", err)
	}
	if err := qtx.DeleteAllIssueOperations(ctx); err != nil {
		return fmt.Errorf("clear issue operations: %w", err)
	}
	if err := qtx.DeleteAllIssuePosts(ctx); err != nil {
		return fmt.Errorf("clear issue posts: %w", err)
	}
	if err := qtx.DeleteAllIssues(ctx); err != nil {
		return fmt.Errorf("clear issues: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM tags"); err != nil {
		return fmt.Errorf("clear tags: %w", err)
	}

	for _, issue := range items {
		record, err := recordFromIssue(issue)
		if err != nil {
			return err
		}

		if err := qtx.InsertIssue(ctx, issuesdb.InsertIssueParams{
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
			return fmt.Errorf("replace issue %q: %w", issue.ID, err)
		}
		if err := syncIssueEmbeddingVector(ctx, tx, record.ID, issue.Embedding); err != nil {
			return err
		}

		if err := insertIssuePosts(ctx, qtx, initialDiscussion(issue)); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace issues: %w", err)
	}

	return nil
}

func (s *PostgresStore) ensureDestructiveResetAllowed(ctx context.Context) error {
	var databaseName string
	if err := s.db.QueryRowContext(ctx, "SELECT current_database()").Scan(&databaseName); err != nil {
		return fmt.Errorf("resolve postgres database for destructive reset: %w", err)
	}

	databaseName = strings.TrimSpace(databaseName)
	if strings.HasSuffix(databaseName, "_test") {
		return nil
	}

	return fmt.Errorf("refusing destructive postgres reset against %q: test databases must end with _test", databaseName)
}

func (s *PostgresStore) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := s.queries.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	tags := make([]Tag, 0, len(rows))
	for _, row := range rows {
		tag, err := tagFromQuery(row)
		if err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return cloneTags(tags), nil
}

func (s *PostgresStore) UpsertTags(ctx context.Context, tags []Tag) error {
	normalized := normalizeCatalogTags(tags)
	if len(normalized) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert tags tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)
	for _, tag := range normalized {
		record, err := recordFromTag(tag)
		if err != nil {
			return err
		}

		if err := qtx.UpsertTag(ctx, issuesdb.UpsertTagParams{
			Name:              record.Name,
			Description:       record.Description,
			CreatedAtUnixNano: record.CreatedAtUnixNano,
			EmbeddingJson:     record.EmbeddingJSON,
		}); err != nil {
			return fmt.Errorf("upsert tag %q: %w", tag.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert tags: %w", err)
	}

	return nil
}

func (s *PostgresStore) UpdateTagSpecificity(ctx context.Context, name string, specificity, llm, embedding *float64, computedAt *time.Time) error {
	params := issuesdb.UpdateTagSpecificityParams{
		Name: name,
	}
	if specificity != nil {
		params.Specificity = sql.NullFloat64{Float64: *specificity, Valid: true}
	}
	if llm != nil {
		params.SpecificityLlm = sql.NullFloat64{Float64: *llm, Valid: true}
	}
	if embedding != nil {
		params.SpecificityEmbedding = sql.NullFloat64{Float64: *embedding, Valid: true}
	}
	if computedAt != nil {
		params.SpecificityComputedAt = sql.NullTime{Time: computedAt.UTC(), Valid: true}
	}
	if err := s.queries.UpdateTagSpecificity(ctx, params); err != nil {
		return fmt.Errorf("update tag specificity %q: %w", name, err)
	}
	return nil
}

func (s *PostgresStore) init(ctx context.Context) error {
	return s.runMigrations(ctx)
}

func (s *PostgresStore) runMigrations(ctx context.Context) error {
	_ = ctx
	dbDriver, err := migratepostgres.WithInstance(s.db, &migratepostgres.Config{
		MigrationsTable: schemaMigrationsTable,
	})
	if err != nil {
		return fmt.Errorf("create postgres migrate driver: %w", err)
	}

	sourceDriver, err := iofs.New(pgMigrationsFS, "pgmigrations")
	if err != nil {
		return fmt.Errorf("open postgres migrations: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("construct postgres migrator: %w", err)
	}

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run postgres migrations: %w", err)
	}
	return nil
}

type issueRecord struct {
	ID                string
	Raw               string
	TagsJSON          json.RawMessage
	CreatedBy         string
	CreatedAtUnixNano int64
	Status            string
	ClosedAtUnixNano  int64
	ClosedBy          string
	TagScoresJSON     json.RawMessage
	EmbeddingJSON     json.RawMessage
	AssignedTo        string
}

type tagRecord struct {
	Name              string
	Description       string
	CreatedAtUnixNano int64
	EmbeddingJSON     json.RawMessage
}

type issueQuerier interface {
	GetIssue(context.Context, string) (issuesdb.GetIssueRow, error)
	ListIssueReferencesByIDs(context.Context, []string) ([]issuesdb.ListIssueReferencesByIDsRow, error)
	ListIssuePosts(context.Context, string) ([]issuesdb.IssuePost, error)
	ListIssueLinksForIssue(context.Context, issuesdb.ListIssueLinksForIssueParams) ([]issuesdb.IssueLink, error)
	ListIssueOperationsForIssue(context.Context, string) ([]issuesdb.IssueOperation, error)
	ListIssueOperationParticipantsForOperations(context.Context, []string) ([]issuesdb.ListIssueOperationParticipantsForOperationsRow, error)
}

type loggingIssueQuerier struct {
	inner  issueQuerier
	logger *slog.Logger
}

func (q loggingIssueQuerier) GetIssue(ctx context.Context, id string) (row issuesdb.GetIssueRow, err error) {
	defer logSQLQueryTiming(ctx, q.logger, "GetIssue", time.Now(), slog.String("issue_id", id))
	return q.inner.GetIssue(ctx, id)
}

func (q loggingIssueQuerier) ListIssueReferencesByIDs(
	ctx context.Context,
	ids []string,
) (rows []issuesdb.ListIssueReferencesByIDsRow, err error) {
	defer func(start time.Time) {
		logSQLQueryTiming(ctx, q.logger, "ListIssueReferencesByIDs", start,
			slog.Int("issue_count", len(ids)),
			slog.Int("row_count", len(rows)),
		)
	}(time.Now())
	return q.inner.ListIssueReferencesByIDs(ctx, ids)
}

func (q loggingIssueQuerier) ListIssuePosts(ctx context.Context, id string) (rows []issuesdb.IssuePost, err error) {
	defer func(start time.Time) {
		logSQLQueryTiming(ctx, q.logger, "ListIssuePosts", start,
			slog.String("issue_id", id),
			slog.Int("row_count", len(rows)),
		)
	}(time.Now())
	return q.inner.ListIssuePosts(ctx, id)
}

func (q loggingIssueQuerier) ListIssueLinksForIssue(
	ctx context.Context,
	arg issuesdb.ListIssueLinksForIssueParams,
) (rows []issuesdb.IssueLink, err error) {
	defer func(start time.Time) {
		logSQLQueryTiming(ctx, q.logger, "ListIssueLinksForIssue", start,
			slog.String("source_issue_id", arg.SourceIssueID),
			slog.String("target_issue_id", arg.TargetIssueID),
			slog.Int("row_count", len(rows)),
		)
	}(time.Now())
	return q.inner.ListIssueLinksForIssue(ctx, arg)
}

func (q loggingIssueQuerier) ListIssueOperationsForIssue(
	ctx context.Context,
	issueID string,
) (rows []issuesdb.IssueOperation, err error) {
	defer func(start time.Time) {
		logSQLQueryTiming(ctx, q.logger, "ListIssueOperationsForIssue", start,
			slog.String("issue_id", issueID),
			slog.Int("row_count", len(rows)),
		)
	}(time.Now())
	return q.inner.ListIssueOperationsForIssue(ctx, issueID)
}

func (q loggingIssueQuerier) ListIssueOperationParticipantsForOperations(
	ctx context.Context,
	operationIDs []string,
) (rows []issuesdb.ListIssueOperationParticipantsForOperationsRow, err error) {
	defer func(start time.Time) {
		logSQLQueryTiming(ctx, q.logger, "ListIssueOperationParticipantsForOperations", start,
			slog.Int("operation_count", len(operationIDs)),
			slog.Int("row_count", len(rows)),
		)
	}(time.Now())
	return q.inner.ListIssueOperationParticipantsForOperations(ctx, operationIDs)
}

func logSQLQueryTiming(ctx context.Context, logger *slog.Logger, queryName string, start time.Time, attrs ...slog.Attr) {
	if logger == nil {
		return
	}
	allAttrs := make([]slog.Attr, 0, len(attrs)+2)
	allAttrs = append(allAttrs, slog.String("query_name", queryName))
	allAttrs = append(allAttrs, slog.Duration("duration", time.Since(start).Round(time.Millisecond)))
	allAttrs = append(allAttrs, attrs...)
	logger.LogAttrs(ctx, slog.LevelInfo, "sql query complete", allAttrs...)
}

func (s *PostgresStore) logServiceTiming(ctx context.Context, serviceName string, start time.Time, attrs ...slog.Attr) {
	if s == nil || s.logger == nil {
		return
	}
	allAttrs := make([]slog.Attr, 0, len(attrs)+2)
	allAttrs = append(allAttrs, slog.String("service", serviceName))
	allAttrs = append(allAttrs, slog.Duration("duration", time.Since(start).Round(time.Millisecond)))
	allAttrs = append(allAttrs, attrs...)
	s.logger.LogAttrs(ctx, slog.LevelInfo, "service call complete", allAttrs...)
}

func recordFromIssue(issue Issue) (issueRecord, error) {
	tagsJSON, err := marshalJSONB(issue.Tags, []string{})
	if err != nil {
		return issueRecord{}, fmt.Errorf("marshal issue tags: %w", err)
	}
	tagScoresJSON, err := marshalJSONB(issue.TagScores, []TagRelevance{})
	if err != nil {
		return issueRecord{}, fmt.Errorf("marshal issue tag scores: %w", err)
	}
	embeddingJSON, err := marshalJSONB(issue.Embedding, []float64{})
	if err != nil {
		return issueRecord{}, fmt.Errorf("marshal issue embedding: %w", err)
	}

	return issueRecord{
		ID:                strings.TrimSpace(issue.ID),
		Raw:               strings.TrimSpace(issue.Raw),
		TagsJSON:          tagsJSON,
		CreatedBy:         strings.TrimSpace(issue.CreatedBy),
		CreatedAtUnixNano: issue.CreatedAt.UTC().UnixNano(),
		Status:            string(normalizeIssueStatus(issue.Status)),
		ClosedAtUnixNano:  closedAtUnixNano(issue),
		ClosedBy:          closedBy(issue),
		TagScoresJSON:     tagScoresJSON,
		EmbeddingJSON:     embeddingJSON,
		AssignedTo:        strings.TrimSpace(issue.AssignedTo),
	}, nil
}

func issueFromQuery(row issuesdb.Issue) (Issue, error) {
	tags, err := unmarshalJSONB[[]string](row.TagsJson)
	if err != nil {
		return Issue{}, fmt.Errorf("decode tags for %q: %w", row.ID, err)
	}
	tagScores, err := unmarshalJSONB[[]TagRelevance](row.TagScoresJson)
	if err != nil {
		return Issue{}, fmt.Errorf("decode tag scores for %q: %w", row.ID, err)
	}
	embedding, err := unmarshalJSONB[[]float64](row.EmbeddingJson)
	if err != nil {
		return Issue{}, fmt.Errorf("decode embedding for %q: %w", row.ID, err)
	}

	status := normalizeIssueStatus(IssueStatus(row.Status))
	closedAt := closedAtFromUnixNano(row.ClosedAtUnixNano)
	closedBy := strings.TrimSpace(row.ClosedBy)
	if status != StatusClosed {
		closedAt = nil
		closedBy = ""
	}

	return normalizeIssueEnrichment(Issue{
		ID:                       row.ID,
		Raw:                      row.Raw,
		Tags:                     tags,
		CreatedBy:                row.CreatedBy,
		CreatedAt:                time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Status:                   status,
		ClosedAt:                 closedAt,
		ClosedBy:                 closedBy,
		AssignedTo:               strings.TrimSpace(row.AssignedTo),
		TagScores:                tagScores,
		Embedding:                embedding,
		EnrichmentStatus:         normalizeIssueEnrichmentStatus(IssueEnrichmentStatus(row.EnrichmentStatus)),
		EnrichmentError:          strings.TrimSpace(row.EnrichmentError),
		EnrichmentTargetSequence: int(row.EnrichmentTargetSequence), //nolint:gosec
	}), nil
}

type issueEnrichmentStateRecord struct {
	Status         IssueEnrichmentStatus
	Error          string
	TargetSequence int
}

func loadIssueEnrichmentStates(
	ctx context.Context,
	db issuesdb.DBTX,
	logger *slog.Logger,
	ids []string,
) (map[string]issueEnrichmentStateRecord, error) {
	if len(ids) == 0 {
		return map[string]issueEnrichmentStateRecord{}, nil
	}

	defer func(start time.Time) {
		logSQLQueryTiming(ctx, logger, "LoadIssueEnrichmentStates", start, slog.Int("issue_count", len(ids)))
	}(time.Now())

	rows, err := db.QueryContext(
		ctx,
		`SELECT id, enrichment_status, enrichment_error, enrichment_target_sequence
		 FROM issues
		 WHERE id = ANY($1)`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("load issue enrichment states: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	states := make(map[string]issueEnrichmentStateRecord, len(ids))
	for rows.Next() {
		var (
			id             string
			status         string
			enrichmentErr  string
			targetSequence int64
		)
		if err := rows.Scan(&id, &status, &enrichmentErr, &targetSequence); err != nil {
			return nil, fmt.Errorf("scan issue enrichment state: %w", err)
		}
		states[id] = issueEnrichmentStateRecord{
			Status:         normalizeIssueEnrichmentStatus(IssueEnrichmentStatus(status)),
			Error:          strings.TrimSpace(enrichmentErr),
			TargetSequence: int(targetSequence), //nolint:gosec
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue enrichment states: %w", err)
	}
	return states, nil
}

func applyIssueEnrichmentStates(items []Issue, states map[string]issueEnrichmentStateRecord) []Issue {
	for i := range items {
		state, ok := states[items[i].ID]
		if !ok {
			items[i] = normalizeIssueEnrichment(items[i])
			continue
		}
		items[i].EnrichmentStatus = state.Status
		items[i].EnrichmentError = state.Error
		items[i].EnrichmentTargetSequence = state.TargetSequence
		items[i] = normalizeIssueEnrichment(items[i])
	}
	return items
}

func issueModelFromGetIssueRow(row issuesdb.GetIssueRow) issuesdb.Issue {
	return issuesdb.Issue{
		ID:                       row.ID,
		Raw:                      row.Raw,
		TagsJson:                 row.TagsJson,
		CreatedBy:                row.CreatedBy,
		CreatedAtUnixNano:        row.CreatedAtUnixNano,
		Status:                   row.Status,
		ClosedAtUnixNano:         row.ClosedAtUnixNano,
		ClosedBy:                 row.ClosedBy,
		TagScoresJson:            row.TagScoresJson,
		EmbeddingJson:            row.EmbeddingJson,
		AssignedTo:               row.AssignedTo,
		EnrichmentStatus:         row.EnrichmentStatus,
		EnrichmentError:          row.EnrichmentError,
		EnrichmentTargetSequence: row.EnrichmentTargetSequence,
	}
}

func issueModelFromListIssuesRow(row issuesdb.ListIssuesRow) issuesdb.Issue {
	return issuesdb.Issue{
		ID:                row.ID,
		Raw:               row.Raw,
		TagsJson:          row.TagsJson,
		CreatedBy:         row.CreatedBy,
		CreatedAtUnixNano: row.CreatedAtUnixNano,
		Status:            row.Status,
		ClosedAtUnixNano:  row.ClosedAtUnixNano,
		ClosedBy:          row.ClosedBy,
		TagScoresJson:     row.TagScoresJson,
		EmbeddingJson:     row.EmbeddingJson,
		AssignedTo:        row.AssignedTo,
	}
}

func issueModelFromListIssuesFilteredRow(row issuesdb.ListIssuesFilteredRow) issuesdb.Issue {
	return issuesdb.Issue{
		ID:                row.ID,
		Raw:               row.Raw,
		TagsJson:          row.TagsJson,
		CreatedBy:         row.CreatedBy,
		CreatedAtUnixNano: row.CreatedAtUnixNano,
		Status:            row.Status,
		ClosedAtUnixNano:  row.ClosedAtUnixNano,
		ClosedBy:          row.ClosedBy,
		TagScoresJson:     row.TagScoresJson,
		EmbeddingJson:     row.EmbeddingJson,
		AssignedTo:        row.AssignedTo,
	}
}

func issueModelFromSearchIssuesByEmbeddingRow(row issuesdb.SearchIssuesByEmbeddingRow) issuesdb.Issue {
	return issuesdb.Issue{
		ID:                row.ID,
		Raw:               row.Raw,
		TagsJson:          row.TagsJson,
		CreatedBy:         row.CreatedBy,
		CreatedAtUnixNano: row.CreatedAtUnixNano,
		Status:            row.Status,
		ClosedAtUnixNano:  row.ClosedAtUnixNano,
		ClosedBy:          row.ClosedBy,
		TagScoresJson:     row.TagScoresJson,
		EmbeddingJson:     row.EmbeddingJson,
		AssignedTo:        row.AssignedTo,
	}
}

func semanticSearchResultsFromFilteredRows(rows []issuesdb.ListIssuesFilteredRow) ([]SemanticSearchResult, error) {
	results := make([]SemanticSearchResult, 0, len(rows))
	for _, row := range rows {
		issue, err := issueFromQuery(issueModelFromListIssuesFilteredRow(row))
		if err != nil {
			return nil, err
		}
		results = append(results, SemanticSearchResult{Issue: issue})
	}
	return results, nil
}

func issuePostFromQuery(row issuesdb.IssuePost) IssuePost {
	return IssuePost{
		ID:        row.ID,
		IssueID:   row.IssueID,
		Raw:       row.Raw,
		CreatedBy: row.CreatedBy,
		CreatedAt: time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Sequence:  int(row.Sequence), //nolint:gosec
		Kind:      row.Kind,
	}
}

func issueLinkFromQuery(row issuesdb.IssueLink) IssueLink {
	return IssueLink{
		ID:            row.ID,
		Type:          normalizeIssueLinkType(IssueLinkType(row.Type)),
		SourceIssueID: row.SourceIssueID,
		TargetIssueID: row.TargetIssueID,
		CreatedBy:     row.CreatedBy,
		CreatedAt:     time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Note:          strings.TrimSpace(row.Note),
		OperationID:   strings.TrimSpace(row.OperationID),
	}
}

func issueOperationFromQuery(row issuesdb.IssueOperation) IssueOperation {
	return IssueOperation{
		ID:        row.ID,
		Kind:      IssueOperationKind(strings.TrimSpace(row.Kind)),
		CreatedBy: row.CreatedBy,
		CreatedAt: time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Note:      strings.TrimSpace(row.Note),
	}
}

func issueOperationParticipantFromQuery(row issuesdb.ListIssueOperationParticipantsForOperationsRow) IssueOperationParticipant {
	return IssueOperationParticipant{
		IssueID: row.IssueID,
		Role:    row.Role,
	}
}

func getIssueDetailBase(ctx context.Context, q issueQuerier, id string) (Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Issue{}, ErrNotFound
	}

	row, err := q.GetIssue(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Issue{}, ErrNotFound
		}
		return Issue{}, err
	}
	return issueFromQuery(issueModelFromGetIssueRow(row))
}

func listIssueDetailPosts(ctx context.Context, q issueQuerier, id string) ([]IssuePost, error) {
	rows, err := q.ListIssuePosts(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("list issue posts: %w", err)
	}

	posts := make([]IssuePost, 0, len(rows))
	for _, row := range rows {
		posts = append(posts, issuePostFromQuery(row))
	}
	return posts, nil
}

func listIssueDetailLinks(ctx context.Context, q issueQuerier, id string) ([]IssueLink, error) {
	id = strings.TrimSpace(id)
	rows, err := q.ListIssueLinksForIssue(ctx, issuesdb.ListIssueLinksForIssueParams{
		SourceIssueID: id,
		TargetIssueID: id,
	})
	if err != nil {
		return nil, fmt.Errorf("list issue links: %w", err)
	}

	links := make([]IssueLink, 0, len(rows))
	for _, row := range rows {
		links = append(links, issueLinkFromQuery(row))
	}
	return links, nil
}

func listIssueDetailOperations(ctx context.Context, q issueQuerier, id string) ([]IssueOperation, error) {
	rows, err := q.ListIssueOperationsForIssue(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("list issue operations: %w", err)
	}

	operationIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		operationIDs = append(operationIDs, row.ID)
	}

	participantsByOperationID := make(map[string][]issuesdb.ListIssueOperationParticipantsForOperationsRow, len(rows))
	if len(operationIDs) > 0 {
		participantRows, err := q.ListIssueOperationParticipantsForOperations(ctx, operationIDs)
		if err != nil {
			return nil, fmt.Errorf("list issue operation participants: %w", err)
		}
		for _, row := range participantRows {
			participantsByOperationID[row.OperationID] = append(participantsByOperationID[row.OperationID], row)
		}
	}

	operations := make([]IssueOperation, 0, len(rows))
	for _, row := range rows {
		operation := issueOperationFromQuery(row)
		participantRows := participantsByOperationID[row.ID]
		participants := make([]IssueOperationParticipant, 0, len(participantRows))
		for _, participantRow := range participantRows {
			participants = append(participants, issueOperationParticipantFromQuery(participantRow))
		}
		operation.Participants = participants
		operations = append(operations, operation)
	}
	return operations, nil
}

func listIssueDetailReferences(ctx context.Context, q issueQuerier, ids []string) ([]IssueReference, error) {
	ids = SanitizeIssueIDs(ids)
	if len(ids) == 0 {
		return []IssueReference{}, nil
	}

	rows, err := q.ListIssueReferencesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list issue references: %w", err)
	}

	refs := make([]IssueReference, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, IssueReference{
			ID:     row.ID,
			Raw:    row.Raw,
			Status: normalizeIssueStatus(IssueStatus(row.Status)),
		})
	}
	return refs, nil
}

func recordFromTag(tag Tag) (tagRecord, error) {
	embeddingJSON, err := marshalJSONB(tag.Embedding, []float64{})
	if err != nil {
		return tagRecord{}, fmt.Errorf("marshal tag embedding: %w", err)
	}

	createdAt := tag.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	return tagRecord{
		Name:              sanitizeTagName(tag.Name),
		Description:       strings.TrimSpace(tag.Description),
		CreatedAtUnixNano: createdAt.UnixNano(),
		EmbeddingJSON:     embeddingJSON,
	}, nil
}

func tagFromQuery(row issuesdb.Tag) (Tag, error) {
	embedding, err := unmarshalJSONB[[]float64](row.EmbeddingJson)
	if err != nil {
		return Tag{}, fmt.Errorf("decode embedding for tag %q: %w", row.Name, err)
	}

	tag := Tag{
		Name:        row.Name,
		Description: row.Description,
		CreatedAt:   time.Unix(0, row.CreatedAtUnixNano).UTC(),
		Embedding:   embedding,
	}

	if row.Specificity.Valid {
		v := row.Specificity.Float64
		tag.Specificity = &v
	}
	if row.SpecificityLlm.Valid {
		v := row.SpecificityLlm.Float64
		tag.SpecificityLLM = &v
	}
	if row.SpecificityEmbedding.Valid {
		v := row.SpecificityEmbedding.Float64
		tag.SpecificityEmbedding = &v
	}
	if row.SpecificityComputedAt.Valid {
		v := row.SpecificityComputedAt.Time.UTC()
		tag.SpecificityComputedAt = &v
	}

	return tag, nil
}

func getIssueWithDiscussion(ctx context.Context, q issueQuerier, id string) (Issue, error) {
	issueRow, err := q.GetIssue(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Issue{}, ErrNotFound
		}
		return Issue{}, err
	}

	postRows, err := q.ListIssuePosts(ctx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("list issue posts: %w", err)
	}
	linkRows, err := q.ListIssueLinksForIssue(ctx, issuesdb.ListIssueLinksForIssueParams{
		SourceIssueID: id,
		TargetIssueID: id,
	})
	if err != nil {
		return Issue{}, fmt.Errorf("list issue links: %w", err)
	}
	operationRows, err := q.ListIssueOperationsForIssue(ctx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("list issue operations: %w", err)
	}
	operationIDs := make([]string, 0, len(operationRows))
	for _, row := range operationRows {
		operationIDs = append(operationIDs, row.ID)
	}
	participantsRows := make([]issuesdb.ListIssueOperationParticipantsForOperationsRow, 0)
	if len(operationIDs) > 0 {
		participantsRows, err = q.ListIssueOperationParticipantsForOperations(ctx, operationIDs)
		if err != nil {
			return Issue{}, fmt.Errorf("list issue operation participants: %w", err)
		}
	}
	issuesByID, err := loadIssueReferenceIndex(ctx, q, issueRow, linkRows, participantsRows)
	if err != nil {
		return Issue{}, err
	}

	discussion := make([]IssuePost, 0, len(postRows))
	for _, row := range postRows {
		discussion = append(discussion, issuePostFromQuery(row))
	}
	links := make([]IssueLink, 0, len(linkRows))
	for _, row := range linkRows {
		links = append(links, issueLinkFromQuery(row))
	}

	participantsByOperationID := make(map[string][]issuesdb.ListIssueOperationParticipantsForOperationsRow, len(operationRows))
	for _, row := range participantsRows {
		participantsByOperationID[row.OperationID] = append(participantsByOperationID[row.OperationID], row)
	}

	operations := make([]IssueOperation, 0, len(operationRows))
	for _, row := range operationRows {
		operation := issueOperationFromQuery(row)
		participantRows := participantsByOperationID[row.ID]
		participants := make([]IssueOperationParticipant, 0, len(participantRows))
		for _, participantRow := range participantRows {
			participant := issueOperationParticipantFromQuery(participantRow)
			if related, ok := issuesByID[participant.IssueID]; ok {
				ref := issueReference(related)
				participant.Issue = &ref
			}
			participants = append(participants, participant)
		}
		operation.Participants = participants
		operations = append(operations, operation)
	}

	return hydrateIssueWithDiscussion(issueRow, discussion, links, operations, issuesByID)
}

func loadIssueReferenceIndex(
	ctx context.Context,
	q issueQuerier,
	issueRow issuesdb.GetIssueRow,
	linkRows []issuesdb.IssueLink,
	participantRows []issuesdb.ListIssueOperationParticipantsForOperationsRow,
) (map[string]Issue, error) {
	currentIssue, err := issueFromQuery(issueModelFromGetIssueRow(issueRow))
	if err != nil {
		return nil, err
	}

	issuesByID := map[string]Issue{
		currentIssue.ID: currentIssue,
	}

	relatedIDs := make([]string, 0, len(linkRows)+len(participantRows))
	seen := map[string]struct{}{
		currentIssue.ID: {},
	}
	addID := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		relatedIDs = append(relatedIDs, id)
	}

	for _, row := range linkRows {
		addID(row.SourceIssueID)
		addID(row.TargetIssueID)
	}
	for _, row := range participantRows {
		addID(row.IssueID)
	}

	if len(relatedIDs) == 0 {
		return issuesByID, nil
	}

	referenceRows, err := q.ListIssueReferencesByIDs(ctx, relatedIDs)
	if err != nil {
		return nil, fmt.Errorf("list issue references: %w", err)
	}
	for _, row := range referenceRows {
		issuesByID[row.ID] = Issue{
			ID:     row.ID,
			Raw:    row.Raw,
			Status: normalizeIssueStatus(IssueStatus(row.Status)),
		}
	}
	return issuesByID, nil
}

func hydrateIssueWithDiscussion(
	issueRow issuesdb.GetIssueRow,
	discussion []IssuePost,
	links []IssueLink,
	operations []IssueOperation,
	issuesByID map[string]Issue,
) (Issue, error) {
	issue, err := issueFromQuery(issueModelFromGetIssueRow(issueRow))
	if err != nil {
		return Issue{}, err
	}

	if len(discussion) == 0 {
		discussion = initialDiscussion(issue)
	}
	issue.Discussion = cloneIssuePosts(discussion)
	for _, link := range links {
		hydrated := link
		if hydrated.SourceIssueID == issue.ID {
			hydrated.Direction = "outgoing"
			if related, ok := issuesByID[hydrated.TargetIssueID]; ok {
				ref := issueReference(related)
				hydrated.RelatedIssue = &ref
			}
		} else {
			hydrated.Direction = "incoming"
			if related, ok := issuesByID[hydrated.SourceIssueID]; ok {
				ref := issueReference(related)
				hydrated.RelatedIssue = &ref
			}
		}
		issue.Links = append(issue.Links, hydrated)
	}
	issue.Operations = cloneIssueOperations(operations)
	return cloneIssues([]Issue{issue})[0], nil
}

func insertIssuePosts(ctx context.Context, q *issuesdb.Queries, posts []IssuePost) error {
	for _, post := range posts {
		if err := q.InsertIssuePost(ctx, issuesdb.InsertIssuePostParams{
			ID:                strings.TrimSpace(post.ID),
			IssueID:           strings.TrimSpace(post.IssueID),
			Raw:               strings.TrimSpace(post.Raw),
			CreatedBy:         strings.TrimSpace(post.CreatedBy),
			CreatedAtUnixNano: post.CreatedAt.UTC().UnixNano(),
			Sequence:          int64(post.Sequence), //nolint:gosec
			Kind:              post.Kind,
		}); err != nil {
			return fmt.Errorf("insert issue post %q: %w", post.ID, err)
		}
	}
	return nil
}

func closedAtUnixNano(issue Issue) int64 {
	if normalizeIssueStatus(issue.Status) != StatusClosed || issue.ClosedAt == nil {
		return 0
	}
	return issue.ClosedAt.UTC().UnixNano()
}

func closedBy(issue Issue) string {
	if normalizeIssueStatus(issue.Status) != StatusClosed {
		return ""
	}
	return strings.TrimSpace(issue.ClosedBy)
}

func closedAtFromUnixNano(value int64) *time.Time {
	if value <= 0 {
		return nil
	}

	closedAt := time.Unix(0, value).UTC()
	return &closedAt
}

func insertIssueOperation(ctx context.Context, q *issuesdb.Queries, operation IssueOperation) error {
	if err := q.InsertIssueOperation(ctx, issuesdb.InsertIssueOperationParams{
		ID:                strings.TrimSpace(operation.ID),
		Kind:              strings.TrimSpace(string(operation.Kind)),
		CreatedBy:         strings.TrimSpace(operation.CreatedBy),
		CreatedAtUnixNano: operation.CreatedAt.UTC().UnixNano(),
		Note:              strings.TrimSpace(operation.Note),
	}); err != nil {
		return fmt.Errorf("insert issue operation %q: %w", operation.ID, err)
	}
	return nil
}

func insertIssueOperationParticipant(
	ctx context.Context,
	q *issuesdb.Queries,
	operationID string,
	participant IssueOperationParticipant,
	sequence int,
) error {
	if err := q.InsertIssueOperationParticipant(ctx, issuesdb.InsertIssueOperationParticipantParams{
		OperationID: strings.TrimSpace(operationID),
		IssueID:     strings.TrimSpace(participant.IssueID),
		Role:        strings.TrimSpace(participant.Role),
		Sequence:    int64(sequence), //nolint:gosec
	}); err != nil {
		return fmt.Errorf("insert issue operation participant for %q: %w", operationID, err)
	}
	return nil
}

func insertIssueLink(ctx context.Context, q *issuesdb.Queries, link IssueLink) error {
	if err := q.InsertIssueLink(ctx, issuesdb.InsertIssueLinkParams{
		ID:                strings.TrimSpace(link.ID),
		SourceIssueID:     strings.TrimSpace(link.SourceIssueID),
		TargetIssueID:     strings.TrimSpace(link.TargetIssueID),
		Type:              strings.TrimSpace(string(normalizeIssueLinkType(link.Type))),
		CreatedBy:         strings.TrimSpace(link.CreatedBy),
		CreatedAtUnixNano: link.CreatedAt.UTC().UnixNano(),
		Note:              strings.TrimSpace(link.Note),
		OperationID:       strings.TrimSpace(link.OperationID),
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23514":
				return fmt.Errorf("%w: sourceId and targetId must differ", ErrInvalidIssueLink)
			case "23505":
				return fmt.Errorf("%w: %s %s %s already exists", ErrDuplicateIssueLink, link.SourceIssueID, link.Type, link.TargetIssueID)
			}
		}
		return fmt.Errorf("insert issue link %q: %w", link.ID, err)
	}
	return nil
}

func marshalJSONB[T any](value T, empty T) (json.RawMessage, error) {
	v := any(value)
	if v == nil {
		v = empty
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func unmarshalJSONB[T any](raw json.RawMessage) (T, error) {
	var result T
	if len(raw) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	return result, nil
}

func normalizeCatalogTags(tags []Tag) []Tag {
	if len(tags) == 0 {
		return nil
	}

	merged := make(map[string]Tag, len(tags))
	for _, raw := range tags {
		name := sanitizeTagName(raw.Name)
		if name == "" {
			continue
		}

		next := Tag{
			Name:        name,
			Description: strings.TrimSpace(raw.Description),
			CreatedAt:   raw.CreatedAt,
			Embedding:   copyEmbedding(raw.Embedding),
		}

		existing, ok := merged[name]
		if !ok {
			merged[name] = next
			continue
		}

		if existing.Description == "" && next.Description != "" {
			existing.Description = next.Description
		}
		if len(existing.Embedding) == 0 && len(next.Embedding) > 0 {
			existing.Embedding = copyEmbedding(next.Embedding)
		}
		if existing.CreatedAt.IsZero() && !next.CreatedAt.IsZero() {
			existing.CreatedAt = next.CreatedAt
		}
		merged[name] = existing
	}

	if len(merged) == 0 {
		return nil
	}

	out := make([]Tag, 0, len(merged))
	for _, tag := range merged {
		out = append(out, tag)
	}
	slices.SortStableFunc(out, func(a, b Tag) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return out
}
