package issues

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"splat/internal/testpostgres"
)

var issuesPostgresHarness struct {
	once    sync.Once
	harness *testpostgres.Harness
	err     error
}

func TestPostgresStoreCreateListAndGet(t *testing.T) {
	store := newPostgresTestStore(t)

	id := NewIssueID()

	issue := BuildNewIssue(id, CreateInput{
		Raw:       "  add postgres storage  ",
		CreatedBy: "  Casey ",
		Tags:      []string{"backend", " backend ", ""},
		TagScores: []TagRelevance{{Tag: "backend", Relevance: 0.9}},
		Embedding: []float64{0.25, 0.5},
	})
	if err := store.SaveIssue(context.Background(), issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	if issue.ID == "" {
		t.Fatal("expected non-empty generated ID")
	}
	if issue.Raw != "add postgres storage" {
		t.Fatalf("expected trimmed raw value, got %q", issue.Raw)
	}
	if issue.CreatedBy != "Casey" {
		t.Fatalf("expected trimmed creator, got %q", issue.CreatedBy)
	}
	if issue.Status != StatusOpen {
		t.Fatalf("expected created issue to be open, got %q", issue.Status)
	}
	if len(issue.Tags) != 1 || issue.Tags[0] != "backend" {
		t.Fatalf("expected sanitized tags, got %#v", issue.Tags)
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

	loaded, err := store.Get(context.Background(), issue.ID)
	if err != nil {
		t.Fatalf("get created issue: %v", err)
	}
	if loaded.ID != issue.ID {
		t.Fatalf("expected %q, got %q", issue.ID, loaded.ID)
	}
	if loaded.Raw != issue.Raw {
		t.Fatalf("expected raw %q, got %q", issue.Raw, loaded.Raw)
	}
	if len(loaded.Discussion) != 1 {
		t.Fatalf("expected initial discussion post, got %#v", loaded.Discussion)
	}
	if loaded.Discussion[0].Raw != issue.Raw {
		t.Fatalf("expected discussion to preserve original raw, got %q", loaded.Discussion[0].Raw)
	}
	assertIssueEmbeddingVectorText(t, store, issue.ID, "[0.25,0.5]")
}

func TestPostgresStoreRefineAppendsDiscussionAndUpdatesCanonicalIssue(t *testing.T) {
	store := newPostgresTestStore(t)

	id := NewIssueID()

	issue := BuildNewIssue(id, CreateInput{
		Raw:       "export fails on ipad",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "export", Relevance: 0.8}},
		Embedding: []float64{0.25, 0.5},
	})
	if err := store.SaveIssue(context.Background(), issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	// Load the issue to get discussion for sequence
	loaded, err := store.Get(context.Background(), issue.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}

	// Create refinement post
	post := NewDiscussionPost(
		issue.ID,
		loaded.Discussion,
		"Customer says this only happens in Safari after tapping share twice.",
		"Jordan",
		"refinement",
	)
	if err := store.SaveIssuePost(context.Background(), post); err != nil {
		t.Fatalf("save issue post: %v", err)
	}

	// Update canonical fields
	canonicalRaw := "Export fails in Safari on iPad after tapping share twice."
	newTags := DisplayTags(nil, []TagRelevance{
		{Tag: "export", Relevance: 0.95},
		{Tag: "safari", Relevance: 0.73},
	})
	if err := store.UpdateIssueFields(context.Background(), issue.ID, IssueFieldUpdate{
		Raw:  &canonicalRaw,
		Tags: newTags,
		TagScores: []TagRelevance{
			{Tag: "export", Relevance: 0.95},
			{Tag: "safari", Relevance: 0.73},
		},
		Embedding: []float64{0.7, 0.2},
	}); err != nil {
		t.Fatalf("update issue fields: %v", err)
	}

	refined, err := store.Get(context.Background(), issue.ID)
	if err != nil {
		t.Fatalf("get refined issue: %v", err)
	}

	if refined.Raw != canonicalRaw {
		t.Fatalf("unexpected canonical raw: %q", refined.Raw)
	}
	if len(refined.Discussion) != 2 {
		t.Fatalf("expected 2 discussion posts, got %#v", refined.Discussion)
	}
	if refined.Discussion[1].CreatedBy != "Jordan" {
		t.Fatalf("expected refinement author Jordan, got %q", refined.Discussion[1].CreatedBy)
	}
	if refined.Discussion[1].Sequence != 2 {
		t.Fatalf("expected refinement sequence 2, got %d", refined.Discussion[1].Sequence)
	}
	if len(refined.Tags) != 2 || refined.Tags[0] != "export" || refined.Tags[1] != "safari" {
		t.Fatalf("unexpected refined tags: %#v", refined.Tags)
	}
	assertIssueEmbeddingVectorText(t, store, issue.ID, "[0.7,0.2]")
}

func TestPostgresStoreCloseAndReopenIssue(t *testing.T) {
	store := newPostgresTestStore(t)

	id := NewIssueID()

	issue := BuildNewIssue(id, CreateInput{Raw: "close me"})
	if err := store.SaveIssue(context.Background(), issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	// Close the issue
	now := time.Now().UTC()
	closedStatus := StatusClosed
	closedBy := "Casey"
	if err := store.UpdateIssueFields(context.Background(), issue.ID, IssueFieldUpdate{
		Status:   &closedStatus,
		ClosedAt: &now,
		ClosedBy: &closedBy,
	}); err != nil {
		t.Fatalf("close issue: %v", err)
	}

	closed, err := store.Get(context.Background(), issue.ID)
	if err != nil {
		t.Fatalf("get closed issue: %v", err)
	}
	if closed.Status != StatusClosed {
		t.Fatalf("expected closed status, got %q", closed.Status)
	}
	if closed.ClosedAt == nil {
		t.Fatal("expected closedAt to be set")
	}
	if closed.ClosedBy != "Casey" {
		t.Fatalf("expected closedBy Casey, got %q", closed.ClosedBy)
	}

	// Reopen the issue
	openStatus := StatusOpen
	if err := store.UpdateIssueFields(context.Background(), issue.ID, IssueFieldUpdate{
		Status: &openStatus,
	}); err != nil {
		t.Fatalf("reopen issue: %v", err)
	}

	reopened, err := store.Get(context.Background(), issue.ID)
	if err != nil {
		t.Fatalf("get reopened issue: %v", err)
	}
	if reopened.Status != StatusOpen {
		t.Fatalf("expected open status after reopen, got %q", reopened.Status)
	}
	if reopened.ClosedAt != nil {
		t.Fatalf("expected closedAt cleared, got %v", reopened.ClosedAt)
	}
	if reopened.ClosedBy != "" {
		t.Fatalf("expected closedBy cleared, got %q", reopened.ClosedBy)
	}
}

func TestPostgresStoreListFiltered(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	alpha := BuildNewIssue("issue-000001", CreateInput{
		Raw:       "Alpha onboarding issue",
		CreatedBy: "Casey",
		Tags:      []string{"onboarding"},
		TagScores: []TagRelevance{{Tag: "onboarding", Relevance: 0.9}},
		Embedding: []float64{0.1, 0.2},
	})
	alpha.AssignedTo = "Casey"
	if err := store.SaveIssue(ctx, alpha); err != nil {
		t.Fatalf("save alpha: %v", err)
	}

	beta := BuildNewIssue("issue-000002", CreateInput{
		Raw:       "Beta export issue",
		CreatedBy: "Jordan",
		Tags:      []string{"export"},
		TagScores: []TagRelevance{{Tag: "export", Relevance: 0.85}},
		Embedding: []float64{0.3, 0.4},
	})
	beta.AssignedTo = "Jordan"
	if err := store.SaveIssue(ctx, beta); err != nil {
		t.Fatalf("save beta: %v", err)
	}

	closedStatus := StatusClosed
	closedBy := "Jordan"
	closedAt := beta.CreatedAt.Add(time.Hour)
	if err := store.UpdateIssueFields(ctx, beta.ID, IssueFieldUpdate{
		Status:   &closedStatus,
		ClosedBy: &closedBy,
		ClosedAt: &closedAt,
	}); err != nil {
		t.Fatalf("close beta: %v", err)
	}

	gamma := BuildNewIssue("issue-000003", CreateInput{
		Raw:       "Gamma backend issue",
		CreatedBy: "Casey",
		Tags:      []string{"backend"},
		TagScores: []TagRelevance{{Tag: "search", Relevance: 0.5}},
		Embedding: []float64{0.5, 0.6},
	})
	gamma.AssignedTo = "Casey"
	if err := store.SaveIssue(ctx, gamma); err != nil {
		t.Fatalf("save gamma: %v", err)
	}

	openOnly, err := store.ListFiltered(ctx, ListOptions{Status: StatusOpen})
	if err != nil {
		t.Fatalf("list open issues: %v", err)
	}
	if len(openOnly) != 2 {
		t.Fatalf("expected 2 open issues, got %d", len(openOnly))
	}

	caseyOnly, err := store.ListFiltered(ctx, ListOptions{AssignedTo: "casey"})
	if err != nil {
		t.Fatalf("list assigned issues: %v", err)
	}
	if len(caseyOnly) != 2 {
		t.Fatalf("expected 2 Casey issues, got %d", len(caseyOnly))
	}

	tagFiltered, err := store.ListFiltered(ctx, ListOptions{Tags: []string{"search"}})
	if err != nil {
		t.Fatalf("list tagged issues: %v", err)
	}
	if len(tagFiltered) != 1 || tagFiltered[0].ID != gamma.ID {
		t.Fatalf("expected gamma for search tag filter, got %#v", tagFiltered)
	}

	paged, err := store.ListFiltered(ctx, ListOptions{Status: "", Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("list paged issues: %v", err)
	}
	if len(paged) != 1 {
		t.Fatalf("expected 1 paged issue, got %d", len(paged))
	}
}

func TestPostgresStoreInitializesPgvectorIssueEmbeddingSchema(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	var extensionName string
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT extname FROM pg_extension WHERE extname = 'vector'`,
	).Scan(&extensionName); err != nil {
		t.Fatalf("query vector extension: %v", err)
	}
	if extensionName != "vector" {
		t.Fatalf("expected vector extension, got %q", extensionName)
	}

	var dataType, udtName string
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT data_type, udt_name
		 FROM information_schema.columns
		 WHERE table_name = 'issues' AND column_name = 'embedding_vector'`,
	).Scan(&dataType, &udtName); err != nil {
		t.Fatalf("query embedding_vector column: %v", err)
	}
	if udtName != "vector" {
		t.Fatalf("expected embedding_vector udt_name vector, got %q (data_type=%q)", udtName, dataType)
	}

	var definition string
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT pg_get_indexdef(indexrelid)
		 FROM pg_stat_user_indexes
		 WHERE indexrelname = 'issues_embedding_vector_cosine_hnsw_idx'`,
	).Scan(&definition); err != nil {
		t.Fatalf("query embedding_vector index: %v", err)
	}
	if !strings.Contains(definition, "USING hnsw") {
		t.Fatalf("expected hnsw index definition, got %q", definition)
	}
	if !strings.Contains(definition, "vector_cosine_ops") {
		t.Fatalf("expected cosine operator class in index definition, got %q", definition)
	}
}

func TestPostgresStoreBackfillsIssueEmbeddingVectorFromJSON(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	id := "issue-backfill-vector"
	embedding := `[0.11,0.22,0.33]`
	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO issues (
			id, raw, tags_json, created_by, created_at_unix_nano, status, closed_at_unix_nano, closed_by,
			tag_scores_json, embedding_json, embedding_vector, assigned_to
		) VALUES (
			$1, 'legacy issue', '[]'::jsonb, 'Casey', 1, 'open', 0, '',
			'[]'::jsonb, $2::jsonb, NULL, ''
		)`,
		id,
		embedding,
	); err != nil {
		t.Fatalf("insert legacy issue row: %v", err)
	}

	if _, err := store.DB().ExecContext(ctx, "UPDATE schema_migrations SET version = 6, dirty = false"); err != nil {
		t.Fatalf("set schema migration version to 6: %v", err)
	}

	if err := store.runMigrations(ctx); err != nil {
		t.Fatalf("rerun migrations for backfill: %v", err)
	}

	assertIssueEmbeddingVectorText(t, store, id, "[0.11,0.22,0.33]")
}

func TestPostgresStoreSearchIssuesByEmbeddingOrdersAndFilters(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	alpha := BuildNewIssue("issue-search-alpha", CreateInput{
		Raw:       "Export fails on Safari",
		CreatedBy: "Casey",
		Tags:      []string{"export"},
		TagScores: []TagRelevance{{Tag: "export", Relevance: 0.95}},
		Embedding: sparseUnitVector(24, 0),
	})
	alpha.AssignedTo = "Casey"
	if err := store.SaveIssue(ctx, alpha); err != nil {
		t.Fatalf("save alpha: %v", err)
	}

	beta := BuildNewIssue("issue-search-beta", CreateInput{
		Raw:       "Export has an edge-case formatting bug",
		CreatedBy: "Casey",
		Tags:      []string{"export"},
		TagScores: []TagRelevance{{Tag: "export", Relevance: 0.8}},
		Embedding: sparseUnitVector(24, 1),
	})
	beta.AssignedTo = "Casey"
	if err := store.SaveIssue(ctx, beta); err != nil {
		t.Fatalf("save beta: %v", err)
	}

	closed := BuildNewIssue("issue-search-closed", CreateInput{
		Raw:       "Closed export issue",
		CreatedBy: "Casey",
		Tags:      []string{"export"},
		TagScores: []TagRelevance{{Tag: "export", Relevance: 0.9}},
		Embedding: sparseUnitVector(24, 0),
	})
	closed.AssignedTo = "Casey"
	if err := store.SaveIssue(ctx, closed); err != nil {
		t.Fatalf("save closed issue: %v", err)
	}
	closedStatus := StatusClosed
	closedBy := "Casey"
	closedAt := closed.CreatedAt.Add(time.Minute)
	if err := store.UpdateIssueFields(ctx, closed.ID, IssueFieldUpdate{
		Status:   &closedStatus,
		ClosedBy: &closedBy,
		ClosedAt: &closedAt,
	}); err != nil {
		t.Fatalf("close issue: %v", err)
	}

	otherAssignee := BuildNewIssue("issue-search-other-assignee", CreateInput{
		Raw:       "Jordan export bug",
		CreatedBy: "Jordan",
		Tags:      []string{"export"},
		TagScores: []TagRelevance{{Tag: "export", Relevance: 0.88}},
		Embedding: sparseUnitVector(24, 0),
	})
	otherAssignee.AssignedTo = "Jordan"
	if err := store.SaveIssue(ctx, otherAssignee); err != nil {
		t.Fatalf("save other assignee issue: %v", err)
	}

	results, err := store.SearchIssues(ctx, SemanticSearchOptions{
		QueryEmbedding: sparseUnitVector(24, 0),
		Status:         StatusOpen,
		AssignedTo:     "casey",
		Tags:           []string{"export"},
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("search issues by embedding: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 filtered search results, got %#v", results)
	}
	if results[0].Issue.ID != alpha.ID || results[1].Issue.ID != beta.ID {
		t.Fatalf("unexpected semantic search ordering: %#v", results)
	}
	if results[0].SemanticDistance > results[1].SemanticDistance {
		t.Fatalf("expected alpha to be at least as close as beta, got %f > %f", results[0].SemanticDistance, results[1].SemanticDistance)
	}
}

func TestPostgresStoreSearchIssuesSupportsCreatedAtFallback(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	older := BuildNewIssue("issue-search-created-old", CreateInput{
		Raw:       "Search filter mismatch on old issue",
		CreatedBy: "Casey",
		Tags:      []string{"search"},
		TagScores: []TagRelevance{{Tag: "search", Relevance: 0.7}},
		Embedding: sparseUnitVector(24, 2),
	})
	older.CreatedAt = time.Unix(10, 0).UTC()
	older.AssignedTo = "Casey"
	if err := store.SaveIssue(ctx, older); err != nil {
		t.Fatalf("save older issue: %v", err)
	}

	newer := BuildNewIssue("issue-search-created-new", CreateInput{
		Raw:       "Newer search ranking issue",
		CreatedBy: "Casey",
		Tags:      []string{"search"},
		TagScores: []TagRelevance{{Tag: "search", Relevance: 0.9}},
		Embedding: sparseUnitVector(24, 3),
	})
	newer.CreatedAt = time.Unix(20, 0).UTC()
	newer.AssignedTo = "Casey"
	if err := store.SaveIssue(ctx, newer); err != nil {
		t.Fatalf("save newer issue: %v", err)
	}

	excluded := BuildNewIssue("issue-search-created-excluded", CreateInput{
		Raw:       "Newest search issue but excluded",
		CreatedBy: "Casey",
		Tags:      []string{"search"},
		TagScores: []TagRelevance{{Tag: "search", Relevance: 0.95}},
		Embedding: sparseUnitVector(24, 4),
	})
	excluded.CreatedAt = time.Unix(30, 0).UTC()
	excluded.AssignedTo = "Casey"
	if err := store.SaveIssue(ctx, excluded); err != nil {
		t.Fatalf("save excluded issue: %v", err)
	}

	results, err := store.SearchIssues(ctx, SemanticSearchOptions{
		Status:     StatusOpen,
		AssignedTo: "Casey",
		Tags:       []string{"search"},
		ExcludeID:  excluded.ID,
		SortBy:     "created_at",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("search issues by created_at: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 created_at search results after exclusion, got %#v", results)
	}
	if results[0].Issue.ID != newer.ID || results[1].Issue.ID != older.ID {
		t.Fatalf("unexpected created_at ordering: %#v", results)
	}
}

func assertIssueEmbeddingVectorText(t *testing.T, store *PostgresStore, id string, want string) {
	t.Helper()

	var got string
	if err := store.DB().QueryRowContext(
		context.Background(),
		`SELECT COALESCE(embedding_vector::text, '') FROM issues WHERE id = $1`,
		id,
	).Scan(&got); err != nil {
		t.Fatalf("query embedding_vector for %s: %v", id, err)
	}
	if got != want {
		t.Fatalf("unexpected embedding_vector for %s: got %q want %q", id, got, want)
	}
}

func TestPostgresStoreMapProjectionRoundTrip(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	payload := []byte(`{"mapIssues":[]}`)
	if err := store.SaveMapProjection(ctx, 7, payload); err != nil {
		t.Fatalf("save map projection: %v", err)
	}

	loaded, err := store.GetMapProjection(ctx, 7)
	if err != nil {
		t.Fatalf("get map projection: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(loaded, &decoded); err != nil {
		t.Fatalf("decode loaded projection: %v", err)
	}
	if issuesPayload, ok := decoded["mapIssues"].([]any); !ok || len(issuesPayload) != 0 {
		t.Fatalf("expected empty mapIssues, got %#v", decoded["mapIssues"])
	}
}

func TestPostgresStoreLoadMapProjectionDataReturnsLinkedIssuesAndTags(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	parent := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "Search ranking quality regressed",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "search", Relevance: 0.9}},
		Embedding: []float64{1, 0},
	})
	child := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "Map projection should slim payloads",
		CreatedBy: "Jordan",
		TagScores: []TagRelevance{{Tag: "backend", Relevance: 0.8}},
		Embedding: []float64{0, 1},
	})
	if err := store.SaveIssue(ctx, parent); err != nil {
		t.Fatalf("save parent issue: %v", err)
	}
	if err := store.SaveIssue(ctx, child); err != nil {
		t.Fatalf("save child issue: %v", err)
	}
	if err := store.SaveLink(ctx, IssueLink{
		ID:            NewOperationID(),
		Type:          IssueLinkTypeDerivedFrom,
		SourceIssueID: child.ID,
		TargetIssueID: parent.ID,
		CreatedBy:     "Jordan",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save link: %v", err)
	}
	if err := store.UpsertTags(ctx, []Tag{
		{Name: "search", Embedding: []float64{1, 0}},
		{Name: "backend", Embedding: []float64{0, 1}},
	}); err != nil {
		t.Fatalf("upsert tags: %v", err)
	}

	items, tags, err := store.LoadMapProjectionData(ctx)
	if err != nil {
		t.Fatalf("load map projection data: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 projection issues, got %d", len(items))
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 projection tags, got %d", len(tags))
	}

	var linked MapProjectionIssue
	for _, item := range items {
		if item.ID == child.ID {
			linked = item
			break
		}
	}
	if linked.ID == "" {
		t.Fatalf("expected child issue %q in projection data", child.ID)
	}
	if len(linked.Links) != 1 {
		t.Fatalf("expected one projection link, got %#v", linked.Links)
	}
	if linked.Links[0].TargetIssueID != parent.ID || linked.Links[0].Type != IssueLinkTypeDerivedFrom {
		t.Fatalf("unexpected projection link: %#v", linked.Links[0])
	}
}

func TestPostgresStoreSaveLinkRejectsSelfLinks(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	issue := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "Search ranking quality regressed",
		CreatedBy: "Casey",
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	err := store.SaveLink(ctx, IssueLink{
		ID:            NewOperationID(),
		Type:          IssueLinkTypeRelatedTo,
		SourceIssueID: issue.ID,
		TargetIssueID: issue.ID,
		CreatedBy:     "Casey",
		CreatedAt:     time.Now().UTC(),
	})
	if !errors.Is(err, ErrInvalidIssueLink) {
		t.Fatalf("expected invalid issue link error, got %v", err)
	}
}

func TestPostgresStoreSaveLinkRejectsDuplicateLogicalLinks(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	source := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "Search ranking quality regressed",
		CreatedBy: "Casey",
	})
	target := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "Map projection should slim payloads",
		CreatedBy: "Jordan",
	})
	if err := store.SaveIssue(ctx, source); err != nil {
		t.Fatalf("save source issue: %v", err)
	}
	if err := store.SaveIssue(ctx, target); err != nil {
		t.Fatalf("save target issue: %v", err)
	}

	link := IssueLink{
		ID:            NewOperationID(),
		Type:          IssueLinkTypeDerivedFrom,
		SourceIssueID: source.ID,
		TargetIssueID: target.ID,
		CreatedBy:     "Jordan",
		CreatedAt:     time.Now().UTC(),
	}
	if err := store.SaveLink(ctx, link); err != nil {
		t.Fatalf("save first link: %v", err)
	}

	link.ID = NewOperationID()
	err := store.SaveLink(ctx, link)
	if !errors.Is(err, ErrDuplicateIssueLink) {
		t.Fatalf("expected duplicate issue link error, got %v", err)
	}
}

func TestPostgresStoreListIssueMetadata(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	issue := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "Projection overlay metadata only",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "backend", Relevance: 0.9}},
		Embedding: []float64{0.2, 0.8},
	})
	issue.AssignedTo = "Jordan"
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	items, err := store.ListIssueMetadata(ctx)
	if err != nil {
		t.Fatalf("list issue metadata: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 metadata issue, got %d", len(items))
	}
	if items[0].ID != issue.ID {
		t.Fatalf("expected issue %q, got %q", issue.ID, items[0].ID)
	}
	if items[0].Raw != issue.Raw {
		t.Fatalf("expected raw %q, got %q", issue.Raw, items[0].Raw)
	}
	if items[0].Status != issue.Status {
		t.Fatalf("expected status %q, got %q", issue.Status, items[0].Status)
	}
	if items[0].AssignedTo != issue.AssignedTo {
		t.Fatalf("expected assignee %q, got %q", issue.AssignedTo, items[0].AssignedTo)
	}
	if len(items[0].Embedding) != 0 {
		t.Fatalf("expected metadata path to omit embeddings, got %#v", items[0].Embedding)
	}
	if len(items[0].TagScores) != 0 {
		t.Fatalf("expected metadata path to omit tag scores, got %#v", items[0].TagScores)
	}
}

func TestPostgresStoreListPeopleAnalytics(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	openIssue := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "Open issue for Avery",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "search", Relevance: 0.9}},
		Embedding: []float64{0.8, 0.2},
	})
	openIssue.AssignedTo = "Avery"

	closedIssue := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "Closed issue for Jordan",
		CreatedBy: "Jordan",
		TagScores: []TagRelevance{{Tag: "backend", Relevance: 0.7}},
		Embedding: []float64{0.1, 0.9},
	})
	closedIssue.AssignedTo = "Jordan"
	closedStatus := StatusClosed
	closedAt := time.Now().UTC()
	closedBy := "Jordan"

	if err := store.SaveIssue(ctx, openIssue); err != nil {
		t.Fatalf("save open issue: %v", err)
	}
	if err := store.SaveIssue(ctx, closedIssue); err != nil {
		t.Fatalf("save closed issue: %v", err)
	}
	if err := store.UpdateIssueFields(ctx, closedIssue.ID, IssueFieldUpdate{
		Status:   &closedStatus,
		ClosedAt: &closedAt,
		ClosedBy: &closedBy,
	}); err != nil {
		t.Fatalf("close issue: %v", err)
	}

	items, err := store.ListPeopleAnalytics(ctx, ListOptions{
		Status:     StatusClosed,
		AssignedTo: "Jordan",
	})
	if err != nil {
		t.Fatalf("list people analytics: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 analytics issue, got %d", len(items))
	}
	if items[0].AssignedTo != "Jordan" {
		t.Fatalf("expected assignee Jordan, got %q", items[0].AssignedTo)
	}
	if items[0].Status != StatusClosed {
		t.Fatalf("expected closed status, got %q", items[0].Status)
	}
	if len(items[0].TagScores) != 1 || items[0].TagScores[0].Tag != "backend" {
		t.Fatalf("unexpected tag scores: %#v", items[0].TagScores)
	}
	if len(items[0].Embedding) != 2 {
		t.Fatalf("expected persisted embedding, got %#v", items[0].Embedding)
	}
}

func TestPostgresStoreListCompareIssues(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	first := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "First compare issue",
		CreatedBy: "Casey",
		Embedding: []float64{1, 0},
	})
	second := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "Second compare issue",
		CreatedBy: "Jordan",
		Embedding: []float64{0.6, 0.8},
	})

	if err := store.SaveIssue(ctx, first); err != nil {
		t.Fatalf("save first issue: %v", err)
	}
	if err := store.SaveIssue(ctx, second); err != nil {
		t.Fatalf("save second issue: %v", err)
	}

	items, err := store.ListCompareIssues(ctx, []string{second.ID, first.ID})
	if err != nil {
		t.Fatalf("list compare issues: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 compare issues, got %d", len(items))
	}
	if items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("expected compare issues in requested order, got %#v", items)
	}
	if len(items[0].Embedding) != 2 || len(items[1].Embedding) != 2 {
		t.Fatalf("expected embeddings to be loaded, got %#v", items)
	}
}

func TestPostgresStorePersistsIssueRelationshipsAndOperationHistory(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	// Create parent issue
	parentID := NewIssueID()
	parent := BuildNewIssue(parentID, CreateInput{
		Raw:       "Large onboarding redesign",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "feature", Relevance: 0.9}},
		Embedding: []float64{0.1, 0.2},
	})
	if err := store.SaveIssue(ctx, parent); err != nil {
		t.Fatalf("save parent issue: %v", err)
	}

	// Create child issues (simulating split)
	child1ID := NewIssueID()
	child1 := BuildNewIssue(child1ID, CreateInput{
		Raw:       "Add onboarding checklist",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "feature", Relevance: 0.8}},
	})
	if err := store.SaveIssue(ctx, child1); err != nil {
		t.Fatalf("save child1: %v", err)
	}

	child2ID := NewIssueID()
	child2 := BuildNewIssue(child2ID, CreateInput{
		Raw:       "Improve invite acceptance copy",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "ux", Relevance: 0.7}},
	})
	if err := store.SaveIssue(ctx, child2); err != nil {
		t.Fatalf("save child2: %v", err)
	}

	// Create split operation
	splitOpID := NewOperationID()
	splitOp := IssueOperation{
		ID:        splitOpID,
		Kind:      IssueOperationKindSplit,
		CreatedBy: "Casey",
		CreatedAt: time.Now().UTC(),
		Note:      "Break the umbrella issue into shippable work.",
		Participants: []IssueOperationParticipant{
			{IssueID: parent.ID, Role: "source"},
			{IssueID: child1.ID, Role: "child"},
			{IssueID: child2.ID, Role: "child"},
		},
	}
	if err := store.SaveOperation(ctx, splitOp); err != nil {
		t.Fatalf("save split operation: %v", err)
	}

	// Create split links
	now := time.Now().UTC()
	splitLinks := []IssueLink{
		{ID: parent.ID + "-" + child1.ID + "-parent", Type: IssueLinkTypeParentOf, SourceIssueID: parent.ID, TargetIssueID: child1.ID, CreatedBy: "Casey", CreatedAt: now, OperationID: splitOpID},
		{ID: child1.ID + "-" + parent.ID + "-child", Type: IssueLinkTypeChildOf, SourceIssueID: child1.ID, TargetIssueID: parent.ID, CreatedBy: "Casey", CreatedAt: now, OperationID: splitOpID},
		{ID: parent.ID + "-" + child2.ID + "-parent", Type: IssueLinkTypeParentOf, SourceIssueID: parent.ID, TargetIssueID: child2.ID, CreatedBy: "Casey", CreatedAt: now, OperationID: splitOpID},
		{ID: child2.ID + "-" + parent.ID + "-child", Type: IssueLinkTypeChildOf, SourceIssueID: child2.ID, TargetIssueID: parent.ID, CreatedBy: "Casey", CreatedAt: now, OperationID: splitOpID},
	}
	for _, link := range splitLinks {
		if err := store.SaveLink(ctx, link); err != nil {
			t.Fatalf("save split link: %v", err)
		}
	}

	// Close parent after split
	closedStatus := StatusClosed
	closedBy := "Casey"
	if err := store.UpdateIssueFields(ctx, parent.ID, IssueFieldUpdate{
		Status:   &closedStatus,
		ClosedAt: &now,
		ClosedBy: &closedBy,
	}); err != nil {
		t.Fatalf("close parent: %v", err)
	}

	// Create link between children
	linkOpID := NewOperationID()
	linkOp := IssueOperation{
		ID:        linkOpID,
		Kind:      IssueOperationKindLink,
		CreatedBy: "Jordan",
		CreatedAt: time.Now().UTC(),
		Note:      "These two child issues ship together.",
		Participants: []IssueOperationParticipant{
			{IssueID: child1.ID, Role: "source"},
			{IssueID: child2.ID, Role: "target"},
		},
	}
	if err := store.SaveOperation(ctx, linkOp); err != nil {
		t.Fatalf("save link operation: %v", err)
	}
	relatedLink := IssueLink{
		ID: child1.ID + "-" + child2.ID + "-related", Type: IssueLinkTypeRelatedTo,
		SourceIssueID: child1.ID, TargetIssueID: child2.ID,
		CreatedBy: "Jordan", CreatedAt: time.Now().UTC(), OperationID: linkOpID,
	}
	if err := store.SaveLink(ctx, relatedLink); err != nil {
		t.Fatalf("save related link: %v", err)
	}

	// Combine children into new issue
	combinedID := NewIssueID()
	combined := BuildNewIssue(combinedID, CreateInput{
		Raw:       "Deliver a tighter onboarding flow with checklist and invite improvements.",
		CreatedBy: "Taylor",
		TagScores: []TagRelevance{{Tag: "feature", Relevance: 0.95}},
		Embedding: []float64{0.3, 0.7},
	})
	if err := store.SaveIssue(ctx, combined); err != nil {
		t.Fatalf("save combined issue: %v", err)
	}

	combineOpID := NewOperationID()
	combineOp := IssueOperation{
		ID:        combineOpID,
		Kind:      IssueOperationKindCombine,
		CreatedBy: "Taylor",
		CreatedAt: time.Now().UTC(),
		Note:      "Roll the child issues into a single delivery artifact.",
		Participants: []IssueOperationParticipant{
			{IssueID: combined.ID, Role: "result"},
			{IssueID: child1.ID, Role: "source"},
			{IssueID: child2.ID, Role: "source"},
		},
	}
	if err := store.SaveOperation(ctx, combineOp); err != nil {
		t.Fatalf("save combine operation: %v", err)
	}

	// Create combine links + close source issues
	combineLinks := []IssueLink{
		{ID: child1.ID + "-" + combined.ID + "-merged", Type: IssueLinkTypeMergedInto, SourceIssueID: child1.ID, TargetIssueID: combined.ID, CreatedBy: "Taylor", CreatedAt: now, OperationID: combineOpID},
		{ID: child2.ID + "-" + combined.ID + "-merged", Type: IssueLinkTypeMergedInto, SourceIssueID: child2.ID, TargetIssueID: combined.ID, CreatedBy: "Taylor", CreatedAt: now, OperationID: combineOpID},
	}
	for _, link := range combineLinks {
		if err := store.SaveLink(ctx, link); err != nil {
			t.Fatalf("save combine link: %v", err)
		}
	}

	// Close source issues
	for _, childID := range []string{child1.ID, child2.ID} {
		if err := store.UpdateIssueFields(ctx, childID, IssueFieldUpdate{
			Status:   &closedStatus,
			ClosedAt: &now,
			ClosedBy: &closedBy,
		}); err != nil {
			t.Fatalf("close child %s: %v", childID, err)
		}
	}

	// Verify parent
	loadedParent, err := store.Get(ctx, parent.ID)
	if err != nil {
		t.Fatalf("get parent issue: %v", err)
	}
	if loadedParent.Status != StatusClosed {
		t.Fatalf("expected parent to be closed after split, got %q", loadedParent.Status)
	}
	if len(loadedParent.Links) != 4 {
		t.Fatalf("expected parent to expose split relationships, got %d links: %#v", len(loadedParent.Links), loadedParent.Links)
	}
	if len(loadedParent.Operations) != 1 {
		t.Fatalf("expected parent to show one split operation, got %#v", loadedParent.Operations)
	}

	// Verify child
	loadedChild, err := store.Get(ctx, child1.ID)
	if err != nil {
		t.Fatalf("get child issue: %v", err)
	}
	if loadedChild.Status != StatusClosed {
		t.Fatalf("expected child to be closed after combine, got %q", loadedChild.Status)
	}
	if len(loadedChild.Operations) != 3 {
		t.Fatalf("expected child to show split, link, and combine operations, got %d: %#v", len(loadedChild.Operations), loadedChild.Operations)
	}
	if loadedChild.Links[0].RelatedIssue == nil {
		t.Fatalf("expected hydrated related issue reference, got %#v", loadedChild.Links[0])
	}

	// Verify combined
	loadedCombined, err := store.Get(ctx, combined.ID)
	if err != nil {
		t.Fatalf("get combined issue: %v", err)
	}
	if loadedCombined.Status != StatusOpen {
		t.Fatalf("expected combined issue to remain open, got %q", loadedCombined.Status)
	}
	if len(loadedCombined.Links) != 2 {
		t.Fatalf("expected combined issue to show merged_into links, got %#v", loadedCombined.Links)
	}
	if len(loadedCombined.Operations) != 1 || loadedCombined.Operations[0].Kind != IssueOperationKindCombine {
		t.Fatalf("expected combined issue to show combine operation, got %#v", loadedCombined.Operations)
	}
}

func TestPostgresStoreReplaceAndCreateWithULID(t *testing.T) {
	store := newPostgresTestStore(t)

	if err := store.Replace(context.Background(), FixtureIssues()); err != nil {
		t.Fatalf("replace issues with seeds: %v", err)
	}

	id := NewIssueID()
	issue := BuildNewIssue(id, CreateInput{Raw: "created after sample load"})
	if err := store.SaveIssue(context.Background(), issue); err != nil {
		t.Fatalf("save issue after replace: %v", err)
	}
	if len(issue.ID) != 26 {
		t.Fatalf("expected ULID (26 chars), got %q", issue.ID)
	}

	if err := store.Replace(context.Background(), nil); err != nil {
		t.Fatalf("clear store: %v", err)
	}

	resetID := NewIssueID()
	resetIssue := BuildNewIssue(resetID, CreateInput{Raw: "created after reset"})
	if err := store.SaveIssue(context.Background(), resetIssue); err != nil {
		t.Fatalf("save issue after reset: %v", err)
	}
	if len(resetIssue.ID) != 26 {
		t.Fatalf("expected ULID (26 chars), got %q", resetIssue.ID)
	}
}

func TestPostgresStoreUpsertAndListTags(t *testing.T) {
	store := newPostgresTestStore(t)

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

func newPostgresTestStore(t *testing.T) *PostgresStore {
	t.Helper()

	databaseURL := issuesHarness(t).Acquire(t, func(ctx context.Context, databaseURL string) error {
		store, err := OpenPostgresStore(ctx, databaseURL)
		if err != nil {
			return err
		}
		return store.Close()
	})

	store, err := OpenPostgresStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close postgres store: %v", err)
		}
	})

	return store
}

func issuesHarness(t *testing.T) *testpostgres.Harness {
	t.Helper()

	issuesPostgresHarness.once.Do(func() {
		issuesPostgresHarness.harness, issuesPostgresHarness.err = testpostgres.Start(context.Background(), "splat_issues_test")
	})
	if issuesPostgresHarness.err != nil {
		t.Fatalf("start postgres test harness: %v", issuesPostgresHarness.err)
	}
	return issuesPostgresHarness.harness
}

func sparseUnitVector(dim, index int) []float64 {
	vector := make([]float64, dim)
	if index >= 0 && index < dim {
		vector[index] = 1
	}
	return vector
}
