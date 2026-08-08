package issues

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenSQLiteDatabaseAppliesMigrationsAndEnablesForeignKeys(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sortit.db")
	database, err := OpenSQLiteDatabase(context.Background(), path)
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var migrationCount int
	if err := database.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 3 {
		t.Fatalf("migration count = %d, want 3", migrationCount)
	}

	if _, err := database.DB().Exec(`INSERT INTO issue_posts (id, issue_id, raw, created_by, created_at_unix_nano, sequence) VALUES ('post_1', 'missing', 'x', 'tester', 1, 1)`); err == nil {
		t.Fatal("foreign-key violation unexpectedly succeeded")
	}

	if _, err := database.DB().Exec(`INSERT INTO issues (id, raw, created_by, created_at_unix_nano, status) VALUES ('issue_1', 'SQLite', 'tester', 1, 'open')`); err != nil {
		t.Fatalf("insert issue: %v", err)
	}
	if _, err := database.DB().Exec(`INSERT INTO issue_posts (id, issue_id, raw, created_by, created_at_unix_nano, sequence) VALUES ('post_1', 'issue_1', 'x', 'tester', 1, 1)`); err != nil {
		t.Fatalf("insert issue post: %v", err)
	}
}

func TestOpenSQLiteDatabaseIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sortit.db")
	database, err := OpenSQLiteDatabase(context.Background(), path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}

	database, err = OpenSQLiteDatabase(context.Background(), path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var version int
	err = database.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != 3 {
		t.Fatalf("migration version = %d, want 3", version)
	}
}

func TestSQLiteStorePersistsIssueGraphAtomically(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "sortit.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	issue := BuildNewIssue("issue_sqlite", CreateInput{
		Raw:       "Persist an issue in SQLite",
		Tags:      []string{"database"},
		CreatedBy: "Taylor",
		Embedding: []float64{0.25, 0.75},
	})
	issue.EnrichmentStatus = EnrichmentStatusPending

	uow, err := store.BeginUnitOfWork(ctx)
	if err != nil {
		t.Fatalf("begin unit of work: %v", err)
	}
	if err := uow.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}
	if err := uow.SaveIssuePost(ctx, IssuePost{
		ID:        "post_sqlite_progress",
		IssueID:   issue.ID,
		Raw:       "Store supports a second post.",
		CreatedBy: "Taylor",
		CreatedAt: issue.CreatedAt.Add(time.Second),
		Sequence:  2,
		Kind:      "progress",
	}); err != nil {
		t.Fatalf("save issue post: %v", err)
	}
	if err := uow.SaveLink(ctx, IssueLink{
		ID:            "link_sqlite",
		Type:          IssueLinkTypeRelatedTo,
		SourceIssueID: issue.ID,
		TargetIssueID: issue.ID,
		CreatedBy:     "Taylor",
		CreatedAt:     issue.CreatedAt,
	}); err != nil {
		t.Fatalf("save link: %v", err)
	}
	if err := uow.SaveOperation(ctx, IssueOperation{
		ID:        "operation_sqlite",
		Kind:      IssueOperationKindLink,
		CreatedBy: "Taylor",
		CreatedAt: issue.CreatedAt,
		Participants: []IssueOperationParticipant{
			{IssueID: issue.ID, Role: "source"},
		},
	}); err != nil {
		t.Fatalf("save operation: %v", err)
	}
	if err := uow.RecordEvent(ctx, issue.ReportEvent()); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if err := uow.Commit(); err != nil {
		t.Fatalf("commit unit of work: %v", err)
	}

	got, err := store.Get(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if got.Raw != issue.Raw || len(got.Discussion) != 2 || len(got.Links) != 1 || len(got.Operations) != 1 {
		t.Fatalf("unexpected persisted issue: %#v", got)
	}
	if len(got.Embedding) != 2 || got.EnrichmentStatus != EnrichmentStatusPending {
		t.Fatalf("SQLite lost enrichment data: %#v", got)
	}
	events, nextCursor, err := store.ListEvents(ctx, 1, "", "report")
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || nextCursor != "" || events[0].IssueID != issue.ID {
		t.Fatalf("unexpected persisted events: %#v (next %q)", events, nextCursor)
	}
}

func TestSQLiteStoreRollsBackIssueMutation(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "sortit.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	uow, err := store.BeginUnitOfWork(ctx)
	if err != nil {
		t.Fatalf("begin unit of work: %v", err)
	}
	issue := BuildNewIssue("issue_rollback", CreateInput{Raw: "Rollback me", CreatedBy: "Taylor"})
	if err := uow.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}
	if err := uow.Rollback(); err != nil {
		t.Fatalf("rollback unit of work: %v", err)
	}
	if _, err := store.Get(ctx, issue.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get rolled-back issue error = %v, want ErrNotFound", err)
	}
}

func TestSQLiteStoreSearchesFTS5ThenEmbeddings(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sortit.db")
	store, err := OpenSQLiteStore(ctx, path)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, issue := range []Issue{
		BuildNewIssue("issue_fts", CreateInput{
			Raw:       "Search needs a local BM25 index",
			Tags:      []string{"search"},
			CreatedBy: "Taylor",
			Embedding: []float64{1, 0},
		}),
		BuildNewIssue("issue_vector", CreateInput{
			Raw:       "Rebuild a projection cache",
			Tags:      []string{"backend"},
			CreatedBy: "Taylor",
			Embedding: []float64{0, 1},
		}),
	} {
		if err := store.SaveIssue(ctx, issue); err != nil {
			t.Fatalf("save issue %q: %v", issue.ID, err)
		}
	}

	textResults, err := store.SearchIssues(ctx, SemanticSearchOptions{
		QueryText: "BM25",
		Status:    StatusOpen,
	})
	if err != nil {
		t.Fatalf("search FTS5: %v", err)
	}
	if len(textResults) != 1 || textResults[0].Issue.ID != "issue_fts" {
		t.Fatalf("FTS5 results = %#v", textResults)
	}

	semanticResults, err := store.SearchIssues(ctx, SemanticSearchOptions{
		QueryText:      "no text match",
		QueryEmbedding: []float64{0.95, 0.05},
		Status:         StatusOpen,
	})
	if err != nil {
		t.Fatalf("search embeddings: %v", err)
	}
	if len(semanticResults) != 2 || semanticResults[0].Issue.ID != "issue_fts" {
		t.Fatalf("semantic results = %#v", semanticResults)
	}

	if err := store.UpdateIssueFields(ctx, "issue_vector", IssueFieldUpdate{Embedding: []float64{0.5, 0.5}}); err != nil {
		t.Fatalf("update indexed embedding: %v", err)
	}
	semanticResults, err = store.SearchIssues(ctx, SemanticSearchOptions{
		QueryEmbedding: []float64{0.7, 0.7},
		Status:         StatusOpen,
	})
	if err != nil {
		t.Fatalf("search rebuilt embedding index: %v", err)
	}
	if len(semanticResults) != 2 || semanticResults[0].Issue.ID != "issue_vector" {
		t.Fatalf("rebuilt semantic results = %#v", semanticResults)
	}

	var persistedIndexes int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM vector_storage WHERE "index" IS NOT NULL AND length("index") > 0`).Scan(&persistedIndexes); err != nil {
		t.Fatalf("count persisted vector indexes: %v", err)
	}
	if persistedIndexes != 1 {
		t.Fatalf("persisted vector indexes = %d, want 1", persistedIndexes)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close sqlite store: %v", err)
	}
	store, err = OpenSQLiteStore(ctx, path)
	if err != nil {
		t.Fatalf("reopen sqlite store: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM vector_storage WHERE "index" IS NOT NULL AND length("index") > 0`).Scan(&persistedIndexes); err != nil {
		t.Fatalf("count reopened vector indexes: %v", err)
	}
	if persistedIndexes != 1 {
		t.Fatalf("reopened persisted vector indexes = %d, want 1", persistedIndexes)
	}
}
