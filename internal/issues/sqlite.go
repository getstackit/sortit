package issues

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	sqliteDriverName = "sqlite"
	issueSeqKey      = "next_issue_seq"
)

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLiteStore(ctx context.Context, path string) (*SQLiteStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("sqlite database path is required")
	}

	if path != ":memory:" {
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if err := ensureDir(dir); err != nil {
				return nil, err
			}
		}
	}

	db, err := sql.Open(sqliteDriverName, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	store := &SQLiteStore{db: db}
	store.db.SetMaxOpenConns(1)

	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) List(ctx context.Context) ([]Issue, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, raw, tags_json, created_by, created_at_unix_nano, tag_scores_json, embedding_json
		FROM issues
		ORDER BY created_at_unix_nano DESC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()

	items := make([]Issue, 0)
	for rows.Next() {
		issue, err := scanIssue(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issues: %w", err)
	}

	return cloneIssues(items), nil
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (Issue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Issue{}, ErrNotFound
	}

	issue, err := scanIssue(s.db.QueryRowContext(ctx, `
		SELECT id, raw, tags_json, created_by, created_at_unix_nano, tag_scores_json, embedding_json
		FROM issues
		WHERE id = ?
	`, id).Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Issue{}, ErrNotFound
		}
		return Issue{}, err
	}

	return cloneIssues([]Issue{issue})[0], nil
}

func (s *SQLiteStore) Create(ctx context.Context, input CreateInput) (Issue, error) {
	raw := strings.TrimSpace(input.Raw)
	if raw == "" {
		return Issue{}, fmt.Errorf("raw is required")
	}

	createdBy := strings.TrimSpace(input.CreatedBy)
	if createdBy == "" {
		createdBy = "You"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issue{}, fmt.Errorf("begin create issue tx: %w", err)
	}
	defer tx.Rollback()

	seq, err := nextSequence(ctx, tx)
	if err != nil {
		return Issue{}, err
	}

	issue := Issue{
		ID:        fmt.Sprintf("issue-%06d", seq),
		Raw:       raw,
		Tags:      displayTags(input.Tags, input.TagScores),
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
		TagScores: copyTagScores(input.TagScores),
		Embedding: copyEmbedding(input.Embedding),
	}

	record, err := recordFromIssue(issue)
	if err != nil {
		return Issue{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO issues (id, raw, tags_json, created_by, created_at_unix_nano, tag_scores_json, embedding_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, record.ID, record.Raw, record.TagsJSON, record.CreatedBy, record.CreatedAtUnixNano, record.TagScoresJSON, record.EmbeddingJSON); err != nil {
		return Issue{}, fmt.Errorf("insert issue: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Issue{}, fmt.Errorf("commit create issue: %w", err)
	}

	return issue, nil
}

func (s *SQLiteStore) Replace(ctx context.Context, next []Issue) error {
	items := cloneIssues(next)
	slices.SortStableFunc(items, compareIssueOrder)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace issues tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM issues`); err != nil {
		return fmt.Errorf("clear issues: %w", err)
	}

	for _, issue := range items {
		record, err := recordFromIssue(issue)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO issues (id, raw, tags_json, created_by, created_at_unix_nano, tag_scores_json, embedding_json)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, record.ID, record.Raw, record.TagsJSON, record.CreatedBy, record.CreatedAtUnixNano, record.TagScoresJSON, record.EmbeddingJSON); err != nil {
			return fmt.Errorf("replace issue %q: %w", issue.ID, err)
		}
	}

	if err := setSequence(ctx, tx, nextSequenceBase(items)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace issues: %w", err)
	}

	return nil
}

func (s *SQLiteStore) init(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("configure sqlite busy timeout: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
		return fmt.Errorf("configure sqlite journal mode: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS issues (
			id TEXT PRIMARY KEY,
			raw TEXT NOT NULL,
			tags_json TEXT NOT NULL,
			created_by TEXT NOT NULL,
			created_at_unix_nano INTEGER NOT NULL,
			tag_scores_json TEXT NOT NULL,
			embedding_json TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create issues table: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create metadata table: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO metadata (key, value)
		VALUES (?, '0')
		ON CONFLICT(key) DO NOTHING
	`, issueSeqKey); err != nil {
		return fmt.Errorf("initialize issue sequence: %w", err)
	}
	return nil
}

type issueRecord struct {
	ID                string
	Raw               string
	TagsJSON          string
	CreatedBy         string
	CreatedAtUnixNano int64
	TagScoresJSON     string
	EmbeddingJSON     string
}

func recordFromIssue(issue Issue) (issueRecord, error) {
	tagsJSON, err := marshalStringSlice(issue.Tags)
	if err != nil {
		return issueRecord{}, fmt.Errorf("marshal issue tags: %w", err)
	}
	tagScoresJSON, err := marshalTagScores(issue.TagScores)
	if err != nil {
		return issueRecord{}, fmt.Errorf("marshal issue tag scores: %w", err)
	}
	embeddingJSON, err := marshalEmbedding(issue.Embedding)
	if err != nil {
		return issueRecord{}, fmt.Errorf("marshal issue embedding: %w", err)
	}

	return issueRecord{
		ID:                strings.TrimSpace(issue.ID),
		Raw:               strings.TrimSpace(issue.Raw),
		TagsJSON:          tagsJSON,
		CreatedBy:         strings.TrimSpace(issue.CreatedBy),
		CreatedAtUnixNano: issue.CreatedAt.UTC().UnixNano(),
		TagScoresJSON:     tagScoresJSON,
		EmbeddingJSON:     embeddingJSON,
	}, nil
}

func scanIssue(scan func(dest ...any) error) (Issue, error) {
	var (
		id                string
		raw               string
		tagsJSON          string
		createdBy         string
		createdAtUnixNano int64
		tagScoresJSON     string
		embeddingJSON     string
	)

	if err := scan(&id, &raw, &tagsJSON, &createdBy, &createdAtUnixNano, &tagScoresJSON, &embeddingJSON); err != nil {
		return Issue{}, fmt.Errorf("scan issue: %w", err)
	}

	createdAt := time.Unix(0, createdAtUnixNano).UTC()

	tags, err := unmarshalStringSlice(tagsJSON)
	if err != nil {
		return Issue{}, fmt.Errorf("decode tags for %q: %w", id, err)
	}
	tagScores, err := unmarshalTagScores(tagScoresJSON)
	if err != nil {
		return Issue{}, fmt.Errorf("decode tag scores for %q: %w", id, err)
	}
	embedding, err := unmarshalEmbedding(embeddingJSON)
	if err != nil {
		return Issue{}, fmt.Errorf("decode embedding for %q: %w", id, err)
	}

	return Issue{
		ID:        id,
		Raw:       raw,
		Tags:      tags,
		CreatedBy: createdBy,
		CreatedAt: createdAt,
		TagScores: tagScores,
		Embedding: embedding,
	}, nil
}

func nextSequence(ctx context.Context, tx *sql.Tx) (uint64, error) {
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = ?`, issueSeqKey).Scan(&raw); err != nil {
		return 0, fmt.Errorf("load issue sequence: %w", err)
	}

	current, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse issue sequence: %w", err)
	}

	next := current + 1
	if err := setSequence(ctx, tx, next); err != nil {
		return 0, err
	}
	return next, nil
}

func setSequence(ctx context.Context, tx *sql.Tx, seq uint64) error {
	if _, err := tx.ExecContext(ctx, `UPDATE metadata SET value = ? WHERE key = ?`, strconv.FormatUint(seq, 10), issueSeqKey); err != nil {
		return fmt.Errorf("update issue sequence: %w", err)
	}
	return nil
}

func nextSequenceBase(items []Issue) uint64 {
	var maxSeq uint64
	if length := uint64(len(items)); length > maxSeq {
		maxSeq = length
	}

	for _, issue := range items {
		seq, ok := issueSequence(issue.ID)
		if ok && seq > maxSeq {
			maxSeq = seq
		}
	}

	return maxSeq
}

func issueSequence(id string) (uint64, bool) {
	if !strings.HasPrefix(id, "issue-") {
		return 0, false
	}
	seq, err := strconv.ParseUint(strings.TrimPrefix(id, "issue-"), 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}

func marshalStringSlice(tags []string) (string, error) {
	if len(tags) == 0 {
		tags = []string{}
	}
	return marshalJSON(tags)
}

func marshalTagScores(scores []TagRelevance) (string, error) {
	if len(scores) == 0 {
		scores = []TagRelevance{}
	}
	return marshalJSON(scores)
}

func marshalEmbedding(values []float64) (string, error) {
	if len(values) == 0 {
		values = []float64{}
	}
	return marshalJSON(values)
}

func marshalJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func unmarshalStringSlice(raw string) ([]string, error) {
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func unmarshalTagScores(raw string) ([]TagRelevance, error) {
	var scores []TagRelevance
	if err := json.Unmarshal([]byte(raw), &scores); err != nil {
		return nil, err
	}
	return scores, nil
}

func unmarshalEmbedding(raw string) ([]float64, error) {
	var values []float64
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create sqlite directory %q: %w", path, err)
	}
	return nil
}
