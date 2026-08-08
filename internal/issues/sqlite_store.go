package issues

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/viant/sqlite-vec/vector"

	"sortit/internal/vectors"
)

// SQLiteStore persists the core issue graph in a local SQLite database. It is
// deliberately SQL-first rather than a translation layer over PostgresStore:
// JSON and embeddings are portable TEXT values and all mutations use SQLite's
// transaction semantics.
type SQLiteStore struct{ database *SQLiteDatabase }

func OpenSQLiteStore(ctx context.Context, dataSource string) (*SQLiteStore, error) {
	database, err := OpenSQLiteDatabase(ctx, dataSource)
	if err != nil {
		return nil, err
	}
	store := &SQLiteStore{database: database}
	if err := store.reindexVectors(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error { return s.database.Close() }
func (s *SQLiteStore) DB() *sql.DB  { return s.database.DB() }

func (s *SQLiteStore) session() sqliteStoreSession               { return sqliteStoreSession{db: s.database.DB()} }
func (s *SQLiteStore) List(ctx context.Context) ([]Issue, error) { return s.session().List(ctx) }
func (s *SQLiteStore) Get(ctx context.Context, id string) (Issue, error) {
	return s.session().Get(ctx, id)
}
func (s *SQLiteStore) GetIssueDetail(ctx context.Context, id string) (Issue, error) {
	return s.Get(ctx, id)
}
func (s *SQLiteStore) GetIssueDetailBase(ctx context.Context, id string) (Issue, error) {
	return s.session().GetIssueDetailBase(ctx, id)
}
func (s *SQLiteStore) ListIssueDetailPosts(ctx context.Context, id string) ([]IssuePost, error) {
	return s.session().ListIssueDetailPosts(ctx, id)
}
func (s *SQLiteStore) ListIssueDetailLinks(ctx context.Context, id string) ([]IssueLink, error) {
	return s.session().ListIssueDetailLinks(ctx, id)
}
func (s *SQLiteStore) ListIssueDetailOperations(ctx context.Context, id string) ([]IssueOperation, error) {
	return s.session().ListIssueDetailOperations(ctx, id)
}
func (s *SQLiteStore) ListIssueDetailReferences(ctx context.Context, ids []string) ([]IssueReference, error) {
	return s.session().ListIssueDetailReferences(ctx, ids)
}
func (s *SQLiteStore) ListIssueSnapshots(context.Context, string) ([]IssueSnapshot, error) {
	return nil, nil
}
func (s *SQLiteStore) SaveIssue(ctx context.Context, issue Issue) error {
	return s.session().SaveIssue(ctx, issue)
}
func (s *SQLiteStore) SaveIssuePost(ctx context.Context, post IssuePost) error {
	return s.session().SaveIssuePost(ctx, post)
}
func (s *SQLiteStore) UpdateIssueFields(ctx context.Context, id string, fields IssueFieldUpdate) error {
	return s.session().UpdateIssueFields(ctx, id, fields)
}
func (s *SQLiteStore) SaveOperation(ctx context.Context, op IssueOperation) error {
	return s.session().SaveOperation(ctx, op)
}
func (s *SQLiteStore) SaveLink(ctx context.Context, link IssueLink) error {
	return s.session().SaveLink(ctx, link)
}
func (s *SQLiteStore) RecordEvent(ctx context.Context, event Event) error {
	return s.session().RecordEvent(ctx, event)
}
func (s *SQLiteStore) ListEvents(ctx context.Context, limit int, cursor, kind string) ([]Event, string, error) {
	return s.session().ListEvents(ctx, limit, cursor, kind)
}
func (s *SQLiteStore) ListLifecycleEvents(ctx context.Context, kinds []string, start, end time.Time) ([]Event, error) {
	return s.session().ListLifecycleEvents(ctx, kinds, start, end)
}
func (s *SQLiteStore) EnqueueIssueEnrichment(ctx context.Context, issueID string, targetSequence int) error {
	return s.session().EnqueueIssueEnrichment(ctx, issueID, targetSequence)
}
func (s *SQLiteStore) SearchIssues(ctx context.Context, opts SemanticSearchOptions) ([]SemanticSearchResult, error) {
	return s.session().SearchIssues(ctx, opts)
}

func (s *SQLiteStore) reindexVectors(ctx context.Context) error {
	items, err := s.List(ctx)
	if err != nil {
		return err
	}
	for _, issue := range items {
		if len(issue.Embedding) == 0 {
			continue
		}
		var indexed bool
		if err := s.DB().QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM _vec_issue_embeddings WHERE dataset_id = ? AND id = ?
		)`, "issues", issue.ID).Scan(&indexed); err != nil {
			return fmt.Errorf("check SQLite vector for %q: %w", issue.ID, err)
		}
		if indexed {
			continue
		}
		if err := s.session().syncVector(ctx, issue.ID); err != nil {
			return fmt.Errorf("index SQLite vector for %q: %w", issue.ID, err)
		}
	}
	return nil
}

func (s *SQLiteStore) BeginUnitOfWork(ctx context.Context) (UnitOfWork, error) {
	tx, err := s.database.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin sqlite unit of work: %w", err)
	}
	return &SQLiteUnitOfWork{sqliteStoreSession: sqliteStoreSession{db: tx}, tx: tx}, nil
}

type SQLiteUnitOfWork struct {
	sqliteStoreSession
	tx *sql.Tx
}

func (u *SQLiteUnitOfWork) Commit() error   { return u.tx.Commit() }
func (u *SQLiteUnitOfWork) Rollback() error { return u.tx.Rollback() }

type sqliteDBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type sqliteStoreSession struct{ db sqliteDBTX }

const sqliteIssueColumns = `id, raw, tags_json, created_by, created_at_unix_nano, status,
closed_at_unix_nano, closed_by, closed_reason, closed_reason_note, tag_scores_json,
embedding_json, assigned_to, enrichment_status, enrichment_error, enrichment_target_sequence`

func (s sqliteStoreSession) List(ctx context.Context) ([]Issue, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+sqliteIssueColumns+" FROM issues ORDER BY created_at_unix_nano DESC, id ASC")
	if err != nil {
		return nil, fmt.Errorf("list sqlite issues: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	items := make([]Issue, 0)
	for rows.Next() {
		issue, err := scanSQLiteIssue(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite issues: %w", err)
	}
	return cloneIssues(items), nil
}

func (s sqliteStoreSession) Get(ctx context.Context, id string) (Issue, error) {
	issue, err := s.GetIssueDetailBase(ctx, id)
	if err != nil {
		return Issue{}, err
	}
	if issue.Discussion, err = s.ListIssueDetailPosts(ctx, id); err != nil {
		return Issue{}, err
	}
	if issue.Links, err = s.ListIssueDetailLinks(ctx, id); err != nil {
		return Issue{}, err
	}
	if issue.Operations, err = s.ListIssueDetailOperations(ctx, id); err != nil {
		return Issue{}, err
	}
	return issue, nil
}

func (s sqliteStoreSession) GetIssueDetailBase(ctx context.Context, id string) (Issue, error) {
	id = strings.TrimSpace(id)
	issue, err := scanSQLiteIssue(s.db.QueryRowContext(ctx, "SELECT "+sqliteIssueColumns+" FROM issues WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	if err != nil {
		return Issue{}, fmt.Errorf("get sqlite issue %q: %w", id, err)
	}
	return issue, nil
}

func (s sqliteStoreSession) ListIssueDetailPosts(ctx context.Context, id string) ([]IssuePost, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, issue_id, raw, created_by, created_at_unix_nano, sequence, kind FROM issue_posts WHERE issue_id = ? ORDER BY sequence, id`, strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("list sqlite issue posts: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var posts []IssuePost
	for rows.Next() {
		var p IssuePost
		var ns int64
		if err := rows.Scan(&p.ID, &p.IssueID, &p.Raw, &p.CreatedBy, &ns, &p.Sequence, &p.Kind); err != nil {
			return nil, fmt.Errorf("scan sqlite issue post: %w", err)
		}
		p.CreatedAt = time.Unix(0, ns).UTC()
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (s sqliteStoreSession) ListIssueDetailLinks(ctx context.Context, id string) ([]IssueLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, type, source_issue_id, target_issue_id, created_by, created_at_unix_nano, note, operation_id FROM issue_links WHERE source_issue_id = ? OR target_issue_id = ? ORDER BY created_at_unix_nano, id`, id, id)
	if err != nil {
		return nil, fmt.Errorf("list sqlite issue links: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var links []IssueLink
	for rows.Next() {
		var l IssueLink
		var ns int64
		if err := rows.Scan(&l.ID, &l.Type, &l.SourceIssueID, &l.TargetIssueID, &l.CreatedBy, &ns, &l.Note, &l.OperationID); err != nil {
			return nil, fmt.Errorf("scan sqlite issue link: %w", err)
		}
		l.CreatedAt = time.Unix(0, ns).UTC()
		links = append(links, l)
	}
	return links, rows.Err()
}

func (s sqliteStoreSession) ListIssueDetailOperations(ctx context.Context, id string) ([]IssueOperation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT o.id, o.kind, o.created_by, o.created_at_unix_nano, o.note FROM issue_operations o JOIN issue_operation_participants p ON p.operation_id = o.id WHERE p.issue_id = ? ORDER BY o.created_at_unix_nano, o.id`, id)
	if err != nil {
		return nil, fmt.Errorf("list sqlite issue operations: %w", err)
	}
	var operations []IssueOperation
	for rows.Next() {
		var op IssueOperation
		var ns int64
		if err := rows.Scan(&op.ID, &op.Kind, &op.CreatedBy, &ns, &op.Note); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan sqlite issue operation: %w", err)
		}
		op.CreatedAt = time.Unix(0, ns).UTC()
		operations = append(operations, op)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// With SQLite's single writer/connection policy, release the operation rows
	// before issuing the participant queries below. Keeping the parent rows open
	// would otherwise starve the nested query from the connection pool.
	for i := range operations {
		op := &operations[i]
		participantRows, err := s.db.QueryContext(ctx, `SELECT issue_id, role FROM issue_operation_participants WHERE operation_id = ? ORDER BY sequence`, op.ID)
		if err != nil {
			return nil, err
		}
		for participantRows.Next() {
			var p IssueOperationParticipant
			if err := participantRows.Scan(&p.IssueID, &p.Role); err != nil {
				_ = participantRows.Close()
				return nil, err
			}
			op.Participants = append(op.Participants, p)
		}
		if err := participantRows.Close(); err != nil {
			return nil, err
		}
	}
	return operations, nil
}

func (s sqliteStoreSession) ListIssueDetailReferences(ctx context.Context, ids []string) ([]IssueReference, error) {
	refs := make([]IssueReference, 0, len(ids))
	for _, id := range SanitizeIssueIDs(ids) {
		issue, err := s.GetIssueDetailBase(ctx, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		refs = append(refs, issueReference(issue))
	}
	return refs, nil
}

func (s sqliteStoreSession) SaveIssue(ctx context.Context, issue Issue) error {
	tags, err := sqliteJSON(issue.Tags)
	if err != nil {
		return err
	}
	scores, err := sqliteJSON(issue.TagScores)
	if err != nil {
		return err
	}
	embedding, err := sqliteJSON(issue.Embedding)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO issues (`+sqliteIssueColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, strings.TrimSpace(issue.ID), strings.TrimSpace(issue.Raw), tags, strings.TrimSpace(issue.CreatedBy), issue.CreatedAt.UTC().UnixNano(), normalizeIssueStatus(issue.Status), unixNano(issue.ClosedAt), strings.TrimSpace(issue.ClosedBy), strings.TrimSpace(issue.ClosedReason), strings.TrimSpace(issue.ClosedReasonNote), scores, embedding, strings.TrimSpace(issue.AssignedTo), normalizeIssueEnrichmentStatus(issue.EnrichmentStatus), strings.TrimSpace(issue.EnrichmentError), max(1, issue.EnrichmentTargetSequence))
	if err != nil {
		return fmt.Errorf("save sqlite issue: %w", err)
	}
	for _, post := range issue.Discussion {
		if err := s.SaveIssuePost(ctx, post); err != nil {
			return err
		}
	}
	if err := s.syncSearch(ctx, issue.ID); err != nil {
		return err
	}
	return s.syncVector(ctx, issue.ID)
}

func (s sqliteStoreSession) SaveIssuePost(ctx context.Context, post IssuePost) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO issue_posts (id, issue_id, raw, created_by, created_at_unix_nano, sequence, kind) VALUES (?, ?, ?, ?, ?, ?, ?)`, strings.TrimSpace(post.ID), strings.TrimSpace(post.IssueID), strings.TrimSpace(post.Raw), strings.TrimSpace(post.CreatedBy), post.CreatedAt.UTC().UnixNano(), post.Sequence, strings.TrimSpace(post.Kind))
	if err != nil {
		return fmt.Errorf("save sqlite issue post: %w", err)
	}
	return s.syncSearch(ctx, post.IssueID)
}

func (s sqliteStoreSession) UpdateIssueFields(ctx context.Context, id string, fields IssueFieldUpdate) error {
	columns, args := make([]string, 0), make([]any, 0)
	add := func(column string, value any) { columns = append(columns, column+" = ?"); args = append(args, value) }
	if fields.Raw != nil {
		add("raw", strings.TrimSpace(*fields.Raw))
	}
	if fields.Tags != nil {
		value, err := sqliteJSON(fields.Tags)
		if err != nil {
			return err
		}
		add("tags_json", value)
	}
	if fields.TagScores != nil {
		value, err := sqliteJSON(fields.TagScores)
		if err != nil {
			return err
		}
		add("tag_scores_json", value)
	}
	if fields.Embedding != nil {
		value, err := sqliteJSON(fields.Embedding)
		if err != nil {
			return err
		}
		add("embedding_json", value)
	}
	if fields.LifecycleCreatedAt != nil {
		add("created_at_unix_nano", fields.LifecycleCreatedAt.UTC().UnixNano())
	}
	if fields.LifecycleCreatedBy != nil {
		add("created_by", strings.TrimSpace(*fields.LifecycleCreatedBy))
	}
	if fields.Status != nil {
		add("status", normalizeIssueStatus(*fields.Status))
	}
	if fields.ClosedAt != nil {
		add("closed_at_unix_nano", fields.ClosedAt.UTC().UnixNano())
	}
	if fields.ClosedBy != nil {
		add("closed_by", strings.TrimSpace(*fields.ClosedBy))
	}
	if fields.ClosedReason != nil {
		add("closed_reason", strings.TrimSpace(*fields.ClosedReason))
	}
	if fields.ClosedReasonNote != nil {
		add("closed_reason_note", strings.TrimSpace(*fields.ClosedReasonNote))
	}
	if fields.AssignedTo != nil {
		add("assigned_to", strings.TrimSpace(*fields.AssignedTo))
	}
	if fields.EnrichmentStatus != nil {
		add("enrichment_status", normalizeIssueEnrichmentStatus(*fields.EnrichmentStatus))
	}
	if fields.EnrichmentError != nil {
		add("enrichment_error", strings.TrimSpace(*fields.EnrichmentError))
	}
	if fields.EnrichmentTargetSequence != nil {
		add("enrichment_target_sequence", max(1, *fields.EnrichmentTargetSequence))
	}
	if len(columns) == 0 {
		return nil
	}
	args = append(args, strings.TrimSpace(id))
	result, err := s.db.ExecContext(ctx, "UPDATE issues SET "+strings.Join(columns, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return fmt.Errorf("update sqlite issue fields: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	if fields.Raw != nil || fields.Tags != nil || fields.TagScores != nil {
		if err := s.syncSearch(ctx, id); err != nil {
			return err
		}
	}
	if fields.Embedding != nil {
		return s.syncVector(ctx, id)
	}
	return nil
}

func (s sqliteStoreSession) SaveOperation(ctx context.Context, op IssueOperation) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO issue_operations (id, kind, created_by, created_at_unix_nano, note) VALUES (?, ?, ?, ?, ?)`, strings.TrimSpace(op.ID), op.Kind, strings.TrimSpace(op.CreatedBy), op.CreatedAt.UTC().UnixNano(), strings.TrimSpace(op.Note)); err != nil {
		return fmt.Errorf("save sqlite issue operation: %w", err)
	}
	for i, participant := range op.Participants {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO issue_operation_participants (operation_id, issue_id, role, sequence) VALUES (?, ?, ?, ?)`, op.ID, participant.IssueID, participant.Role, i+1); err != nil {
			return fmt.Errorf("save sqlite operation participant: %w", err)
		}
	}
	return nil
}
func (s sqliteStoreSession) SaveLink(ctx context.Context, link IssueLink) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO issue_links (id, source_issue_id, target_issue_id, type, created_by, created_at_unix_nano, note, operation_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, link.ID, link.SourceIssueID, link.TargetIssueID, link.Type, link.CreatedBy, link.CreatedAt.UTC().UnixNano(), link.Note, link.OperationID)
	if err != nil {
		return fmt.Errorf("save sqlite issue link: %w", err)
	}
	return nil
}

// SearchIssues first uses FTS5 to reproduce the database-side text candidate
// path provided by ParadeDB. If text finds nothing, it falls back to a local
// cosine scan over stored embeddings. The scan is intentionally bounded by the
// local SQLite deployment model; sqlite-vec can later replace it as an optional
// acceleration without changing this interface.
func (s sqliteStoreSession) SearchIssues(ctx context.Context, opts SemanticSearchOptions) ([]SemanticSearchResult, error) {
	if strings.EqualFold(strings.TrimSpace(opts.SortBy), "created_at") {
		return s.filteredSearchResults(ctx, opts)
	}
	queryText := strings.TrimSpace(opts.QueryText)
	if queryText != "" {
		matches, err := s.searchText(ctx, queryText)
		if err != nil {
			return nil, err
		}
		results := filterSQLiteSearchResults(matches, opts)
		if len(results) > 0 {
			return paginateSQLiteSearchResults(results, opts), nil
		}
	}
	return s.filteredSearchResults(ctx, opts)
}

func (s sqliteStoreSession) searchText(ctx context.Context, queryText string) ([]SemanticSearchResult, error) {
	ftsQuery := sqliteFTSQuery(queryText)
	if ftsQuery == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT issue_id FROM issue_search WHERE issue_search MATCH ? ORDER BY bm25(issue_search), issue_id`, ftsQuery)
	if err != nil {
		return nil, fmt.Errorf("search SQLite FTS5: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan SQLite FTS5 result: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate SQLite FTS5 results: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	results := make([]SemanticSearchResult, 0, len(ids))
	for _, id := range ids {
		issue, err := s.GetIssueDetailBase(ctx, id)
		if err != nil {
			return nil, err
		}
		results = append(results, SemanticSearchResult{Issue: issue})
	}
	return results, nil
}

func (s sqliteStoreSession) filteredSearchResults(ctx context.Context, opts SemanticSearchOptions) ([]SemanticSearchResult, error) {
	if len(opts.QueryEmbedding) > 0 {
		results, err := s.searchVectors(ctx, opts.QueryEmbedding, sqliteVectorCandidateLimit(opts))
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return paginateSQLiteSearchResults(filterSQLiteSearchResults(results, opts), opts), nil
		}
	}
	issues, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]SemanticSearchResult, 0, len(issues))
	for _, issue := range issues {
		if !matchesSQLiteSearchFilters(issue, opts) {
			continue
		}
		if len(opts.QueryEmbedding) > 0 {
			if len(issue.Embedding) != len(opts.QueryEmbedding) {
				continue
			}
			similarity := vectors.CosineSimilarity(issue.Embedding, opts.QueryEmbedding)
			results = append(results, SemanticSearchResult{Issue: issue, SemanticDistance: 1 - similarity})
			continue
		}
		results = append(results, SemanticSearchResult{Issue: issue})
	}
	if len(opts.QueryEmbedding) > 0 {
		sort.SliceStable(results, func(i, j int) bool {
			if results[i].SemanticDistance != results[j].SemanticDistance {
				return results[i].SemanticDistance < results[j].SemanticDistance
			}
			return results[i].Issue.ID < results[j].Issue.ID
		})
	}
	return paginateSQLiteSearchResults(results, opts), nil
}

func (s sqliteStoreSession) searchVectors(ctx context.Context, query []float64, limit int) ([]SemanticSearchResult, error) {
	embedding, err := sqliteVectorBlob(query)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT issue_id, match_score
		FROM issue_embeddings
		WHERE dataset_id = ? AND issue_id MATCH ?
		LIMIT ?`, "issues", embedding, limit)
	if err != nil {
		return nil, fmt.Errorf("search SQLite vector index: %w", err)
	}
	type vectorMatch struct {
		id    string
		score float64
	}
	matches := make([]vectorMatch, 0, limit)
	for rows.Next() {
		var match vectorMatch
		if err := rows.Scan(&match.id, &match.score); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan SQLite vector result: %w", err)
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	results := make([]SemanticSearchResult, 0, len(matches))
	for _, match := range matches {
		issue, err := s.GetIssueDetailBase(ctx, match.id)
		if err != nil {
			return nil, err
		}
		results = append(results, SemanticSearchResult{Issue: issue, SemanticDistance: 1 - match.score})
	}
	return results, nil
}

func sqliteVectorCandidateLimit(opts SemanticSearchOptions) int {
	window := max(opts.Limit+max(0, opts.Offset), 32)
	return min(window*6, 256)
}

func filterSQLiteSearchResults(results []SemanticSearchResult, opts SemanticSearchOptions) []SemanticSearchResult {
	filtered := results[:0]
	for _, result := range results {
		if matchesSQLiteSearchFilters(result.Issue, opts) {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func matchesSQLiteSearchFilters(issue Issue, opts SemanticSearchOptions) bool {
	if opts.Status != "" && issue.Status != opts.Status {
		return false
	}
	if assignee := strings.TrimSpace(opts.AssignedTo); assignee != "" && !strings.EqualFold(issue.AssignedTo, assignee) {
		return false
	}
	if exclude := strings.TrimSpace(opts.ExcludeID); exclude != "" && issue.ID == exclude {
		return false
	}
	if len(opts.Tags) == 0 {
		return true
	}
	for _, filter := range opts.Tags {
		for _, tag := range issue.Tags {
			if strings.EqualFold(strings.TrimSpace(filter), tag) {
				return true
			}
		}
		for _, score := range issue.TagScores {
			if score.EffectiveRelevance() >= 0.3 && strings.EqualFold(strings.TrimSpace(filter), score.Tag) {
				return true
			}
		}
	}
	return false
}

func paginateSQLiteSearchResults(results []SemanticSearchResult, opts SemanticSearchOptions) []SemanticSearchResult {
	offset := max(0, opts.Offset)
	if offset >= len(results) {
		return []SemanticSearchResult{}
	}
	results = results[offset:]
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results
}

func sqliteFTSQuery(query string) string {
	terms := strings.Fields(query)
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.Trim(strings.TrimSpace(term), `"'`)
		if term == "" {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " OR ")
}

func (s sqliteStoreSession) syncSearch(ctx context.Context, issueID string) error {
	issue, err := s.GetIssueDetailBase(ctx, issueID)
	if err != nil {
		return err
	}
	posts, err := s.ListIssueDetailPosts(ctx, issueID)
	if err != nil {
		return err
	}
	bodyParts := make([]string, 0, len(posts))
	for _, post := range posts {
		if raw := strings.TrimSpace(post.Raw); raw != "" {
			bodyParts = append(bodyParts, raw)
		}
	}
	body := strings.Join(bodyParts, "\n\n")
	if body == "" {
		body = issue.Raw
	}
	tags := append([]string(nil), issue.Tags...)
	for _, score := range issue.TagScores {
		if score.EffectiveRelevance() >= 0.3 {
			tags = append(tags, score.Tag)
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM issue_search WHERE issue_id = ?`, issue.ID); err != nil {
		return fmt.Errorf("clear SQLite FTS5 row: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO issue_search (issue_id, title, body, tags) VALUES (?, ?, ?, ?)`, issue.ID, issue.Raw, body, strings.Join(tags, " ")); err != nil {
		return fmt.Errorf("index SQLite FTS5 row: %w", err)
	}
	return nil
}

func (s sqliteStoreSession) syncVector(ctx context.Context, issueID string) error {
	issue, err := s.GetIssueDetailBase(ctx, issueID)
	if err != nil {
		return err
	}
	if len(issue.Embedding) == 0 {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM _vec_issue_embeddings WHERE dataset_id = ? AND id = ?`, "issues", issue.ID); err != nil {
			return fmt.Errorf("remove SQLite vector: %w", err)
		}
		return nil
	}
	embedding, err := sqliteVectorBlob(issue.Embedding)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO _vec_issue_embeddings (dataset_id, id, content, meta, embedding)
		VALUES (?, ?, ?, '{}', ?)
		ON CONFLICT(dataset_id, id) DO UPDATE SET content = excluded.content, embedding = excluded.embedding`, "issues", issue.ID, issue.Raw, embedding); err != nil {
		return fmt.Errorf("index SQLite vector: %w", err)
	}
	return nil
}

func sqliteVectorBlob(values []float64) ([]byte, error) {
	if len(values) == 0 {
		return nil, nil
	}
	encoded := make([]float32, len(values))
	for i, value := range values {
		encoded[i] = float32(value)
	}
	vector, err := vector.EncodeEmbedding(encoded)
	if err != nil {
		return nil, fmt.Errorf("encode SQLite vector: %w", err)
	}
	return vector, nil
}

func (s sqliteStoreSession) RecordEvent(ctx context.Context, event Event) error {
	participants, err := sqliteJSON(event.Participants)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO events (id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json) VALUES (?, ?, ?, ?, ?, ?, ?)`, event.ID, event.Kind, event.IssueID, event.CreatedBy, event.CreatedAt.UTC().UnixNano(), event.Body, participants)
	return err
}
func (s sqliteStoreSession) ListEvents(ctx context.Context, limit int, cursor, kind string) ([]Event, string, error) {
	if limit <= 0 {
		limit = 40
	}
	where, args := make([]string, 0, 2), make([]any, 0, 4)
	if kind = strings.TrimSpace(kind); kind != "" {
		where, args = append(where, "kind = ?"), append(args, kind)
	}
	if cursorAt, cursorID, ok := decodeEventCursor(cursor); ok {
		where, args = append(where, "(created_at_unix_nano < ? OR (created_at_unix_nano = ? AND id < ?))"), append(args, cursorAt.UnixNano(), cursorAt.UnixNano(), cursorID)
	}
	query := `SELECT id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json FROM events`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at_unix_nano DESC, id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list sqlite events: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	events := make([]Event, 0, limit)
	for rows.Next() {
		event, err := scanSQLiteEvent(rows)
		if err != nil {
			return nil, "", err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(events) <= limit {
		return events, "", nil
	}
	events = events[:limit]
	return events, encodeEventCursor(events[len(events)-1]), nil
}

func (s sqliteStoreSession) ListLifecycleEvents(ctx context.Context, kinds []string, start, end time.Time) ([]Event, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	placeholders := make([]string, 0, len(kinds))
	args := make([]any, 0, len(kinds)+2)
	for _, kind := range kinds {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		placeholders, args = append(placeholders, "?"), append(args, kind)
	}
	if len(placeholders) == 0 {
		return nil, nil
	}
	args = append(args, start.UTC().UnixNano(), end.UTC().UnixNano())
	query := `SELECT id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json
		FROM events WHERE kind IN (` + strings.Join(placeholders, ", ") + `)
		AND created_at_unix_nano BETWEEN ? AND ? AND issue_id != ''
		ORDER BY created_at_unix_nano, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sqlite lifecycle events: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var events []Event
	for rows.Next() {
		event, err := scanSQLiteEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
func (s sqliteStoreSession) EnqueueIssueEnrichment(ctx context.Context, issueID string, targetSequence int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO issue_enrichment_jobs (issue_id, target_sequence, attempt_count, available_at_unix_nano, leased_until_unix_nano) VALUES (?, ?, 0, ?, 0) ON CONFLICT(issue_id) DO UPDATE SET target_sequence = MAX(target_sequence, excluded.target_sequence), available_at_unix_nano = excluded.available_at_unix_nano, leased_until_unix_nano = 0`, strings.TrimSpace(issueID), targetSequence, time.Now().UTC().UnixNano())
	return err
}

type sqliteScanner interface{ Scan(...any) error }

func scanSQLiteIssue(row sqliteScanner) (Issue, error) {
	var issue Issue
	var tags, scores, embedding string
	var createdAt, closedAt int64
	var status, enrichment string
	if err := row.Scan(&issue.ID, &issue.Raw, &tags, &issue.CreatedBy, &createdAt, &status, &closedAt, &issue.ClosedBy, &issue.ClosedReason, &issue.ClosedReasonNote, &scores, &embedding, &issue.AssignedTo, &enrichment, &issue.EnrichmentError, &issue.EnrichmentTargetSequence); err != nil {
		return Issue{}, err
	}
	if err := json.Unmarshal([]byte(tags), &issue.Tags); err != nil {
		return Issue{}, fmt.Errorf("decode sqlite tags: %w", err)
	}
	if err := json.Unmarshal([]byte(scores), &issue.TagScores); err != nil {
		return Issue{}, fmt.Errorf("decode sqlite tag scores: %w", err)
	}
	if err := json.Unmarshal([]byte(embedding), &issue.Embedding); err != nil {
		return Issue{}, fmt.Errorf("decode sqlite embedding: %w", err)
	}
	issue.CreatedAt = time.Unix(0, createdAt).UTC()
	issue.Status = normalizeIssueStatus(IssueStatus(status))
	issue.EnrichmentStatus = normalizeIssueEnrichmentStatus(IssueEnrichmentStatus(enrichment))
	if closedAt > 0 {
		at := time.Unix(0, closedAt).UTC()
		issue.ClosedAt = &at
	}
	return issue, nil
}
func scanSQLiteEvent(row sqliteScanner) (Event, error) {
	var event Event
	var createdAt int64
	var participants string
	if err := row.Scan(&event.ID, &event.Kind, &event.IssueID, &event.CreatedBy, &createdAt, &event.Body, &participants); err != nil {
		return Event{}, fmt.Errorf("scan sqlite event: %w", err)
	}
	if err := json.Unmarshal([]byte(participants), &event.Participants); err != nil {
		return Event{}, fmt.Errorf("decode sqlite event participants: %w", err)
	}
	event.CreatedAt = time.Unix(0, createdAt).UTC()
	return event, nil
}
func sqliteJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal sqlite JSON: %w", err)
	}
	return string(data), nil
}
func unixNano(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.UTC().UnixNano()
}

var _ Store = (*SQLiteStore)(nil)
var _ UnitOfWorkBeginner = (*SQLiteStore)(nil)
var _ EventStore = (*SQLiteStore)(nil)
var _ SemanticSearchStore = (*SQLiteStore)(nil)
