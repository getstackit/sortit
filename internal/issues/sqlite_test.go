package issues

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStoreCreateListAndGet(t *testing.T) {
	store := newSQLiteTestStore(t)

	created, err := store.Create(context.Background(), CreateInput{
		Raw:       "  add sqlite storage  ",
		CreatedBy: "  Casey ",
		Tags:      []string{"backend", " backend ", ""},
		TagScores: []TagRelevance{{Tag: "backend", Relevance: 0.9}},
		Embedding: []float64{0.25, 0.5},
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	if created.ID != "issue-000001" {
		t.Fatalf("expected first generated ID, got %q", created.ID)
	}
	if created.Raw != "add sqlite storage" {
		t.Fatalf("expected trimmed raw value, got %q", created.Raw)
	}
	if created.CreatedBy != "Casey" {
		t.Fatalf("expected trimmed creator, got %q", created.CreatedBy)
	}
	if len(created.Tags) != 1 || created.Tags[0] != "backend" {
		t.Fatalf("expected sanitized tags, got %#v", created.Tags)
	}

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 stored issue, got %d", len(items))
	}
	if len(items[0].TagScores) != 1 {
		t.Fatalf("expected persisted tag scores, got %#v", items[0].TagScores)
	}
	if len(items[0].Embedding) != 2 {
		t.Fatalf("expected persisted embedding, got %#v", items[0].Embedding)
	}

	loaded, err := store.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get created issue: %v", err)
	}
	if loaded.ID != created.ID {
		t.Fatalf("expected %q, got %q", created.ID, loaded.ID)
	}
	if loaded.Raw != created.Raw {
		t.Fatalf("expected raw %q, got %q", created.Raw, loaded.Raw)
	}
}

func TestSQLiteStoreReplaceResetsSequenceFromLoadedItems(t *testing.T) {
	store := newSQLiteTestStore(t)

	if err := store.Replace(context.Background(), SeedIssues()); err != nil {
		t.Fatalf("replace issues with seeds: %v", err)
	}

	created, err := store.Create(context.Background(), CreateInput{
		Raw: "created after sample load",
	})
	if err != nil {
		t.Fatalf("create issue after replace: %v", err)
	}
	if created.ID != "issue-000007" {
		t.Fatalf("expected sequence to continue after seeded items, got %q", created.ID)
	}

	if err := store.Replace(context.Background(), nil); err != nil {
		t.Fatalf("clear store: %v", err)
	}

	resetCreated, err := store.Create(context.Background(), CreateInput{
		Raw: "created after reset",
	})
	if err != nil {
		t.Fatalf("create issue after reset: %v", err)
	}
	if resetCreated.ID != "issue-000001" {
		t.Fatalf("expected sequence reset after clearing store, got %q", resetCreated.ID)
	}
}

func TestSQLiteStoreUpsertAndListTags(t *testing.T) {
	store := newSQLiteTestStore(t)

	if err := store.UpsertTags(context.Background(), []Tag{
		{Name: "bug", Description: "software defect", Embedding: []float64{0.2, 0.8}},
		{Name: "bug", Embedding: []float64{0.2, 0.8}},
		{Name: "ux", Description: "usability", Embedding: []float64{0.4, 0.6}},
	}); err != nil {
		t.Fatalf("upsert tags: %v", err)
	}

	tags, err := store.ListTags(context.Background())
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	if tags[0].Name != "bug" {
		t.Fatalf("expected tags sorted by name, got %q first", tags[0].Name)
	}
	if len(tags[0].Embedding) != 2 {
		t.Fatalf("expected persisted tag embedding, got %#v", tags[0].Embedding)
	}
}

func TestSQLiteStoreMigratesLegacyCreatedAtColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.sqlite")

	db, err := sql.Open(sqliteDriverName, dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite db: %v", err)
	}

	legacyCreatedAt := time.Date(2026, time.March, 7, 23, 33, 16, 673615000, time.UTC)
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			raw TEXT NOT NULL,
			tags_json TEXT NOT NULL,
			created_by TEXT NOT NULL,
			created_at TEXT NOT NULL,
			tag_scores_json TEXT NOT NULL,
			embedding_json TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create legacy issues table: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create metadata table: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE tags (
			name TEXT PRIMARY KEY,
			description TEXT NOT NULL,
			created_at_unix_nano INTEGER NOT NULL,
			embedding_json TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create tags table: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO metadata (key, value) VALUES (?, '1')
	`, issueSeqKey); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO issues (id, raw, tags_json, created_by, created_at, tag_scores_json, embedding_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "issue-000001", "legacy issue", `["bug"]`, "Casey", legacyCreatedAt.Format(time.RFC3339Nano), `[]`, `[0.1,0.2]`); err != nil {
		t.Fatalf("seed legacy issue: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy sqlite db: %v", err)
	}

	store, err := OpenSQLiteStore(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open migrated sqlite store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close migrated sqlite store: %v", err)
		}
	})

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list migrated issues: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 migrated issue, got %d", len(items))
	}
	if !items[0].CreatedAt.Equal(legacyCreatedAt) {
		t.Fatalf("expected created_at %s, got %s", legacyCreatedAt, items[0].CreatedAt)
	}

	created, err := store.Create(context.Background(), CreateInput{Raw: "new issue after migration"})
	if err != nil {
		t.Fatalf("create issue after migration: %v", err)
	}
	if created.ID != "issue-000002" {
		t.Fatalf("expected issue-000002 after migration, got %q", created.ID)
	}
}

func TestSQLiteStoreBaselinesCurrentSchemaWithoutReapplyingLegacyMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "current.sqlite")

	db, err := sql.Open(sqliteDriverName, dbPath)
	if err != nil {
		t.Fatalf("open current sqlite db: %v", err)
	}

	currentCreatedAt := time.Date(2026, time.March, 8, 0, 1, 2, 345678000, time.UTC)
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			raw TEXT NOT NULL,
			tags_json TEXT NOT NULL,
			created_by TEXT NOT NULL,
			created_at_unix_nano INTEGER NOT NULL,
			tag_scores_json TEXT NOT NULL,
			embedding_json TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create current issues table: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create current metadata table: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE tags (
			name TEXT PRIMARY KEY,
			description TEXT NOT NULL,
			created_at_unix_nano INTEGER NOT NULL,
			embedding_json TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create current tags table: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO metadata (key, value) VALUES (?, '1')
	`, issueSeqKey); err != nil {
		t.Fatalf("seed current metadata: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO issues (id, raw, tags_json, created_by, created_at_unix_nano, tag_scores_json, embedding_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "issue-000001", "current issue", `["bug"]`, "Casey", currentCreatedAt.UnixNano(), `[]`, `[0.1,0.2]`); err != nil {
		t.Fatalf("seed current issue: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close current sqlite db: %v", err)
	}

	store, err := OpenSQLiteStore(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open current sqlite store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close current sqlite store: %v", err)
		}
	})

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list current issues: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 current issue, got %d", len(items))
	}
	if !items[0].CreatedAt.Equal(currentCreatedAt) {
		t.Fatalf("expected created_at %s, got %s", currentCreatedAt, items[0].CreatedAt)
	}

	created, err := store.Create(context.Background(), CreateInput{Raw: "new issue after baseline"})
	if err != nil {
		t.Fatalf("create issue after baseline: %v", err)
	}
	if created.ID != "issue-000002" {
		t.Fatalf("expected issue-000002 after baseline, got %q", created.ID)
	}
}

func newSQLiteTestStore(t *testing.T) *SQLiteStore {
	t.Helper()

	store, err := OpenSQLiteStore(context.Background(), filepath.Join(t.TempDir(), "issues.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close sqlite store: %v", err)
		}
	})
	return store
}
