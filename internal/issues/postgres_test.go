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
	if issue.CreatedBy != "Casey" { //nolint:goconst
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
	if refined.Discussion[1].CreatedBy != "Jordan" { //nolint:goconst
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

func TestPostgresStorePersistsIssueSnapshotsOnEnrichmentUpdate(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	issue := BuildNewIssue(NewIssueID(), CreateInput{
		Raw:       "Export fails on iPad.",
		CreatedBy: "Casey",
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	raw1 := "Export fails on iPad in Safari."
	seq1 := 1
	if err := store.UpdateIssueFields(ctx, issue.ID, IssueFieldUpdate{
		Raw:  &raw1,
		Tags: []string{"export", "safari"},
		TagScores: []TagRelevance{
			{Tag: "export", Relevance: 0.9},
			{Tag: "safari", Relevance: 0.7},
		},
		Embedding:                []float64{1, 0},
		EnrichmentTargetSequence: &seq1,
	}); err != nil {
		t.Fatalf("update issue fields seq1: %v", err)
	}

	raw2 := "Export fails on iPad in Safari after tapping share twice."
	seq2 := 2
	if err := store.UpdateIssueFields(ctx, issue.ID, IssueFieldUpdate{
		Raw:  &raw2,
		Tags: []string{"export", "safari"},
		TagScores: []TagRelevance{
			{Tag: "export", Relevance: 0.95},
			{Tag: "safari", Relevance: 0.8},
		},
		Embedding:                []float64{0.8, 0.2},
		EnrichmentTargetSequence: &seq2,
	}); err != nil {
		t.Fatalf("update issue fields seq2: %v", err)
	}

	snapshots, err := store.ListIssueSnapshots(ctx, issue.ID)
	if err != nil {
		t.Fatalf("list issue snapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %#v", snapshots)
	}
	if snapshots[0].Sequence != 1 || snapshots[0].Raw != raw1 {
		t.Fatalf("unexpected first snapshot: %#v", snapshots[0])
	}
	if snapshots[1].Sequence != 2 || snapshots[1].Raw != raw2 {
		t.Fatalf("unexpected second snapshot: %#v", snapshots[1])
	}
	if len(snapshots[1].Embedding) != 2 || snapshots[1].Embedding[0] != 0.8 || snapshots[1].Embedding[1] != 0.2 {
		t.Fatalf("unexpected snapshot embedding: %#v", snapshots[1].Embedding)
	}
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

func TestPostgresStoreLifecycleProjectionReadCutover(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	issue := BuildNewIssue("issue-lifecycle-cutover", CreateInput{
		Raw:       "Read lifecycle state from projection",
		CreatedBy: "Casey",
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	legacyClosedAt := issue.CreatedAt.Add(time.Hour)
	if _, err := store.DB().ExecContext(
		ctx,
		`UPDATE issues
		 SET status = 'closed',
		     closed_at_unix_nano = $1,
		     closed_by = 'Legacy',
		     closed_reason = 'fixed',
		     closed_reason_note = 'stale row',
		     assigned_to = 'Legacy'
		 WHERE id = $2`,
		legacyClosedAt.UTC().UnixNano(),
		issue.ID,
	); err != nil {
		t.Fatalf("desync legacy issue row: %v", err)
	}

	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO issue_lifecycle_projections (
		     issue_id,
		     created_by,
		     created_at_unix_nano,
		     status,
		     closed_at_unix_nano,
		     closed_by,
		     closed_reason,
		     closed_reason_note,
		     assigned_to,
		     last_fact_id,
		     fact_count,
		     updated_at_unix_nano
		 ) VALUES ($1, $2, $3, $4, 0, '', '', '', $5, $6, $7, $8)
		 ON CONFLICT (issue_id) DO UPDATE
		 SET created_by = EXCLUDED.created_by,
		     created_at_unix_nano = EXCLUDED.created_at_unix_nano,
		     status = EXCLUDED.status,
		     closed_at_unix_nano = EXCLUDED.closed_at_unix_nano,
		     closed_by = EXCLUDED.closed_by,
		     closed_reason = EXCLUDED.closed_reason,
		     closed_reason_note = EXCLUDED.closed_reason_note,
		     assigned_to = EXCLUDED.assigned_to,
		     last_fact_id = EXCLUDED.last_fact_id,
		     fact_count = EXCLUDED.fact_count,
		     updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		issue.ID,
		issue.CreatedBy,
		issue.CreatedAt.UTC().UnixNano(),
		string(StatusOpen),
		"Jordan",
		"projection-fact",
		1,
		time.Now().UTC().UnixNano(),
	); err != nil {
		t.Fatalf("seed lifecycle projection: %v", err)
	}

	loaded, err := store.Get(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get issue through cutover: %v", err)
	}
	if loaded.Status != StatusOpen {
		t.Fatalf("expected projection status open, got %q", loaded.Status)
	}
	if loaded.AssignedTo != "Jordan" {
		t.Fatalf("expected projection assignee Jordan, got %q", loaded.AssignedTo)
	}
	if loaded.ClosedAt != nil {
		t.Fatalf("expected projection closed_at cleared, got %v", loaded.ClosedAt)
	}
	if loaded.ClosedBy != "" || loaded.ClosedReason != "" || loaded.ClosedReasonNote != "" {
		t.Fatalf("expected projection close fields cleared, got %#v", loaded)
	}

	openItems, err := store.ListFiltered(ctx, ListOptions{Status: StatusOpen})
	if err != nil {
		t.Fatalf("list open issues: %v", err)
	}
	if len(openItems) != 1 || openItems[0].ID != issue.ID {
		t.Fatalf("expected projection-backed issue in open filter, got %#v", openItems)
	}

	closedItems, err := store.ListFiltered(ctx, ListOptions{Status: StatusClosed})
	if err != nil {
		t.Fatalf("list closed issues: %v", err)
	}
	if len(closedItems) != 0 {
		t.Fatalf("expected no closed issues after projection cutover, got %#v", closedItems)
	}
}

func TestPostgresStoreContentProjectionReadCutover(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	issue := BuildNewIssue("issue-content-cutover", CreateInput{
		Raw:       "legacy content should not win",
		CreatedBy: "Casey",
		Tags:      []string{"bug"},
		TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.4}},
		Embedding: []float64{0.1, 0.9},
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	if _, err := store.DB().ExecContext(
		ctx,
		`UPDATE issues
		 SET raw = $2,
		     tags_json = $3::jsonb,
		     tag_scores_json = $4::jsonb,
		     embedding_vector = $5::vector
		 WHERE id = $1`,
		issue.ID,
		"stale legacy row",
		`["legacy"]`,
		`[{"tag":"legacy","relevance":0.2}]`,
		"[1,0]",
	); err != nil {
		t.Fatalf("desync legacy issue row: %v", err)
	}

	if _, err := store.DB().ExecContext(
		ctx,
		`INSERT INTO issue_content_projections (
		     issue_id, raw, tags_json, tag_scores_json, embedding_vector, last_event_id, event_count, updated_at_unix_nano
		 ) VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::vector, $6, $7, $8)
		 ON CONFLICT (issue_id) DO UPDATE
		 SET raw = EXCLUDED.raw,
		     tags_json = EXCLUDED.tags_json,
		     tag_scores_json = EXCLUDED.tag_scores_json,
		     embedding_vector = EXCLUDED.embedding_vector,
		     last_event_id = EXCLUDED.last_event_id,
		     event_count = EXCLUDED.event_count,
		     updated_at_unix_nano = EXCLUDED.updated_at_unix_nano`,
		issue.ID,
		"projection content",
		`["projection"]`,
		`[{"tag":"projection","relevance":0.95}]`,
		"[0,1]",
		"fact-projection",
		99,
		time.Now().UTC().UnixNano(),
	); err != nil {
		t.Fatalf("seed issue content projection: %v", err)
	}

	loaded, err := store.Get(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get issue through cutover: %v", err)
	}
	if loaded.Raw != "projection content" {
		t.Fatalf("expected content projection raw, got %q", loaded.Raw)
	}
	if len(loaded.Tags) != 1 || loaded.Tags[0] != "projection" {
		t.Fatalf("expected content projection tags, got %#v", loaded.Tags)
	}
	if len(loaded.TagScores) != 1 || loaded.TagScores[0].Tag != "projection" {
		t.Fatalf("expected content projection tag scores, got %#v", loaded.TagScores)
	}
	if len(loaded.Embedding) != 2 || loaded.Embedding[0] != 0 || loaded.Embedding[1] != 1 {
		t.Fatalf("expected content projection embedding, got %#v", loaded.Embedding)
	}
}

func TestPostgresStoreLifecycleWritesAppendFactsAndProjection(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	issue := BuildNewIssue("issue-lifecycle-writes", CreateInput{
		Raw:       "Move lifecycle writes to append-only path",
		CreatedBy: "Casey",
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	assignActor := "Jordan"
	assignAt := issue.CreatedAt.Add(time.Minute)
	assignee := "Jordan"
	if err := store.UpdateIssueFields(ctx, issue.ID, IssueFieldUpdate{
		AssignedTo:         &assignee,
		LifecycleCreatedBy: &assignActor,
		LifecycleCreatedAt: &assignAt,
	}); err != nil {
		t.Fatalf("assign issue: %v", err)
	}

	closeActor := "Taylor"
	closeAt := issue.CreatedAt.Add(2 * time.Minute)
	closeStatus := StatusClosed
	reason := "fixed"
	reasonNote := "landed elsewhere"
	if err := store.UpdateIssueFields(ctx, issue.ID, IssueFieldUpdate{
		LifecycleCreatedBy: &closeActor,
		LifecycleCreatedAt: &closeAt,
		Status:             &closeStatus,
		ClosedAt:           &closeAt,
		ClosedBy:           &closeActor,
		ClosedReason:       &reason,
		ClosedReasonNote:   &reasonNote,
	}); err != nil {
		t.Fatalf("close issue: %v", err)
	}

	reopenAt := issue.CreatedAt.Add(3 * time.Minute)
	openStatus := StatusOpen
	if err := store.UpdateIssueFields(ctx, issue.ID, IssueFieldUpdate{
		LifecycleCreatedAt: &reopenAt,
		Status:             &openStatus,
	}); err != nil {
		t.Fatalf("reopen issue: %v", err)
	}

	loaded, err := store.Get(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get issue after lifecycle writes: %v", err)
	}
	if loaded.Status != StatusOpen {
		t.Fatalf("expected reopened issue to read as open, got %q", loaded.Status)
	}
	if loaded.AssignedTo != "Jordan" {
		t.Fatalf("expected projection assignee Jordan, got %q", loaded.AssignedTo)
	}

	var (
		factCount    int
		projStatus   string
		projAssignee string
	)
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM issue_lifecycle_facts WHERE issue_id = $1`,
		issue.ID,
	).Scan(&factCount); err != nil {
		t.Fatalf("count lifecycle facts: %v", err)
	}
	if factCount != 4 {
		t.Fatalf("expected 4 lifecycle facts, got %d", factCount)
	}

	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT status, assigned_to
		 FROM issue_lifecycle_projections
		 WHERE issue_id = $1`,
		issue.ID,
	).Scan(&projStatus, &projAssignee); err != nil {
		t.Fatalf("read lifecycle projection: %v", err)
	}
	if projStatus != string(StatusOpen) {
		t.Fatalf("expected projection status open, got %q", projStatus)
	}
	if projAssignee != "Jordan" {
		t.Fatalf("expected projection assignee Jordan, got %q", projAssignee)
	}

	var legacyAssignedTo string
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT assigned_to FROM issues WHERE id = $1`,
		issue.ID,
	).Scan(&legacyAssignedTo); err != nil {
		t.Fatalf("read legacy issue row: %v", err)
	}
	if legacyAssignedTo != "" {
		t.Fatalf("expected legacy issue lifecycle column to remain untouched, got %q", legacyAssignedTo)
	}
}

func TestPostgresStoreContentWritesAppendFactsAndProjection(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	issue := BuildNewIssue("issue-content-writes", CreateInput{
		Raw:       "original content",
		CreatedBy: "Casey",
		Tags:      []string{"bug"},
		TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.4}},
		Embedding: []float64{0.1, 0.9},
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	raw := "refined canonical content"
	if err := store.UpdateIssueFields(ctx, issue.ID, IssueFieldUpdate{
		Raw:       &raw,
		Tags:      []string{"bug", "ux"},
		TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.7}, {Tag: "ux", Relevance: 0.6}},
		Embedding: []float64{0.3, 0.7},
	}); err != nil {
		t.Fatalf("update issue content fields: %v", err)
	}

	var (
		contentRaw string
		eventCount int
	)
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT raw, event_count FROM issue_content_projections WHERE issue_id = $1`,
		issue.ID,
	).Scan(&contentRaw, &eventCount); err != nil {
		t.Fatalf("read issue content projection: %v", err)
	}
	if contentRaw != raw || eventCount != 2 {
		t.Fatalf("unexpected content projection raw=%q eventCount=%d", contentRaw, eventCount)
	}

	var factCount int
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM issue_content_facts WHERE issue_id = $1`,
		issue.ID,
	).Scan(&factCount); err != nil {
		t.Fatalf("count issue content facts: %v", err)
	}
	if factCount != 2 {
		t.Fatalf("expected 2 issue content facts, got %d", factCount)
	}

	var legacyRaw string
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT raw FROM issues WHERE id = $1`,
		issue.ID,
	).Scan(&legacyRaw); err != nil {
		t.Fatalf("read legacy issue row: %v", err)
	}
	if legacyRaw != "original content" {
		t.Fatalf("expected legacy issue row to remain unchanged, got %q", legacyRaw)
	}
}

func TestPostgresStoreSnapshotsAreInsertOnlyPerSequence(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	issue := BuildNewIssue("issue-snapshot-insert-only", CreateInput{
		Raw:       "Original snapshot candidate",
		CreatedBy: "Casey",
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	raw1 := "First snapshot body"
	seq1 := 1
	if err := store.UpdateIssueFields(ctx, issue.ID, IssueFieldUpdate{
		Raw:                      &raw1,
		Tags:                     []string{"export"},
		TagScores:                []TagRelevance{{Tag: "export", Relevance: 0.9}},
		Embedding:                []float64{1, 0},
		EnrichmentTargetSequence: &seq1,
	}); err != nil {
		t.Fatalf("write first snapshot: %v", err)
	}

	raw2 := "Second snapshot body should be ignored for same sequence"
	if err := store.UpdateIssueFields(ctx, issue.ID, IssueFieldUpdate{
		Raw:                      &raw2,
		Tags:                     []string{"export", "safari"},
		TagScores:                []TagRelevance{{Tag: "safari", Relevance: 0.8}},
		Embedding:                []float64{0, 1},
		EnrichmentTargetSequence: &seq1,
	}); err != nil {
		t.Fatalf("attempt duplicate snapshot write: %v", err)
	}

	snapshots, err := store.ListIssueSnapshots(ctx, issue.ID)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot row, got %#v", snapshots)
	}
	if snapshots[0].Raw != raw1 {
		t.Fatalf("expected first snapshot to win, got %q", snapshots[0].Raw)
	}
	if len(snapshots[0].Tags) != 1 || snapshots[0].Tags[0] != "export" {
		t.Fatalf("expected original snapshot tags to be preserved, got %#v", snapshots[0].Tags)
	}
}

func TestPostgresStoreEnrichmentProjectionReadAndWriteCutover(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	issue := BuildNewIssue("issue-enrichment-cutover", CreateInput{
		Raw:       "Use enrichment projection for reads",
		CreatedBy: "Casey",
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	pendingStatus := EnrichmentStatusPending
	emptyError := ""
	targetSequence := 2
	if err := store.UpdateIssueFields(ctx, issue.ID, IssueFieldUpdate{
		EnrichmentStatus:         &pendingStatus,
		EnrichmentError:          &emptyError,
		EnrichmentTargetSequence: &targetSequence,
	}); err != nil {
		t.Fatalf("set pending enrichment state: %v", err)
	}
	if err := store.EnqueueIssueEnrichment(ctx, issue.ID, targetSequence); err != nil {
		t.Fatalf("enqueue enrichment job: %v", err)
	}

	if _, err := store.DB().ExecContext(
		ctx,
		`UPDATE issues
		 SET enrichment_status = 'complete',
		     enrichment_error = 'legacy stale state',
		     enrichment_target_sequence = 1
		 WHERE id = $1`,
		issue.ID,
	); err != nil {
		t.Fatalf("desync legacy enrichment columns: %v", err)
	}

	loaded, err := store.Get(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get issue through enrichment cutover: %v", err)
	}
	if loaded.EnrichmentStatus != EnrichmentStatusPending {
		t.Fatalf("expected projection status pending, got %q", loaded.EnrichmentStatus)
	}
	if loaded.EnrichmentError != "" {
		t.Fatalf("expected projection error to be empty, got %q", loaded.EnrichmentError)
	}
	if loaded.EnrichmentTargetSequence != targetSequence {
		t.Fatalf("expected projection target sequence %d, got %d", targetSequence, loaded.EnrichmentTargetSequence)
	}

	job, ok, err := store.ClaimNextIssueEnrichment(ctx, time.Minute)
	if err != nil {
		t.Fatalf("claim enrichment job: %v", err)
	}
	if !ok || job.TargetSequence != targetSequence {
		t.Fatalf("unexpected claimed job %#v", job)
	}

	var (
		projStatus       string
		projTarget       int
		projAttemptCount int
		projLease        int64
	)
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT status, target_sequence, attempt_count, lease_expires_at_unix_nano
		 FROM issue_enrichment_projections
		 WHERE issue_id = $1`,
		issue.ID,
	).Scan(&projStatus, &projTarget, &projAttemptCount, &projLease); err != nil {
		t.Fatalf("read enrichment projection: %v", err)
	}
	if projStatus != string(EnrichmentStatusPending) || projTarget != targetSequence {
		t.Fatalf("unexpected enrichment projection status=%q target=%d", projStatus, projTarget)
	}
	if projAttemptCount != 1 || projLease == 0 {
		t.Fatalf("expected claimed projection attempt/lease to be set, got attempt=%d lease=%d", projAttemptCount, projLease)
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

	var pgSearchExtension string
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT extname FROM pg_extension WHERE extname = 'pg_search'`,
	).Scan(&pgSearchExtension); err != nil {
		t.Fatalf("query pg_search extension: %v", err)
	}
	if pgSearchExtension != "pg_search" {
		t.Fatalf("expected pg_search extension, got %q", pgSearchExtension)
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

	var bm25Definition string
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT pg_get_indexdef(indexrelid)
		 FROM pg_stat_user_indexes
		 WHERE indexrelname = 'issue_content_projections_search_bm25_idx'`,
	).Scan(&bm25Definition); err != nil {
		t.Fatalf("query pg_search bm25 index: %v", err)
	}
	if !strings.Contains(bm25Definition, "USING bm25") {
		t.Fatalf("expected bm25 index definition, got %q", bm25Definition)
	}
}

func TestPostgresStoreEmbeddingVectorRoundTrip(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	id := "issue-embedding-rt"
	embedding := []float64{0.11, 0.22, 0.33}
	issue := BuildNewIssue(id, CreateInput{
		Raw:       "embedding round-trip",
		CreatedBy: "Casey",
		Embedding: embedding,
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
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

func TestPostgresStoreSearchIssuesPrefersParadeDBTextMatches(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	textHit := BuildNewIssue("issue-search-text-hit", CreateInput{
		Raw:       "Authority canonicality scoring needs clearer semantics",
		CreatedBy: "Casey",
		Tags:      []string{"search"},
		TagScores: []TagRelevance{{Tag: "search", Relevance: 0.9}},
		Embedding: sparseUnitVector(24, 5),
	})
	if err := store.SaveIssue(ctx, textHit); err != nil {
		t.Fatalf("save textHit: %v", err)
	}

	textMiss := BuildNewIssue("issue-search-text-miss", CreateInput{
		Raw:       "Graph leverage ranking needs more explanation",
		CreatedBy: "Jordan",
		Tags:      []string{"search"},
		TagScores: []TagRelevance{{Tag: "search", Relevance: 0.9}},
		Embedding: sparseUnitVector(24, 5),
	})
	if err := store.SaveIssue(ctx, textMiss); err != nil {
		t.Fatalf("save textMiss: %v", err)
	}

	results, err := store.SearchIssues(ctx, SemanticSearchOptions{
		QueryText:      "authority canonicality",
		QueryEmbedding: sparseUnitVector(24, 5),
		Status:         StatusOpen,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("search issues by text: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected text search results, got none")
	}
	if results[0].Issue.ID != textHit.ID {
		t.Fatalf("expected text-matching issue first, got %#v", results)
	}
}

func TestPostgresStoreSearchIssuesFallsBackToEmbeddingWhenTextMisses(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	alpha := BuildNewIssue("issue-search-fallback-alpha", CreateInput{
		Raw:       "Safari export regression in PDF flow",
		CreatedBy: "Casey",
		Tags:      []string{"export"},
		TagScores: []TagRelevance{{Tag: "export", Relevance: 0.95}},
		Embedding: sparseUnitVector(24, 6),
	})
	if err := store.SaveIssue(ctx, alpha); err != nil {
		t.Fatalf("save alpha: %v", err)
	}

	beta := BuildNewIssue("issue-search-fallback-beta", CreateInput{
		Raw:       "Analytics backfill is too slow",
		CreatedBy: "Jordan",
		Tags:      []string{"backend"},
		TagScores: []TagRelevance{{Tag: "backend", Relevance: 0.95}},
		Embedding: sparseUnitVector(24, 7),
	})
	if err := store.SaveIssue(ctx, beta); err != nil {
		t.Fatalf("save beta: %v", err)
	}

	results, err := store.SearchIssues(ctx, SemanticSearchOptions{
		QueryText:      "zzzz-no-text-hit",
		QueryEmbedding: sparseUnitVector(24, 6),
		Status:         StatusOpen,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("search issues with embedding fallback: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected embedding fallback results, got none")
	}
	if results[0].Issue.ID != alpha.ID {
		t.Fatalf("expected embedding-nearest issue first after text miss, got %#v", results)
	}
}

func TestPostgresStoreMaintainsPgSearchWriteColumns(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	issue := BuildNewIssue("issue-search-write-model", CreateInput{
		Raw:       "Export fails on iPad",
		CreatedBy: "Casey",
		Tags:      []string{"export"},
		TagScores: []TagRelevance{{Tag: "export", Relevance: 0.95}},
		Embedding: []float64{0.2, 0.8},
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	assertIssueSearchProjection(t, store, issue.ID, "Export fails on iPad", "Export fails on iPad", "export")

	post := NewDiscussionPost(
		issue.ID,
		issue.Discussion,
		"Customer says it breaks after tapping share twice.",
		"Jordan",
		"refinement",
	)
	if err := store.SaveIssuePost(ctx, post); err != nil {
		t.Fatalf("save issue post: %v", err)
	}

	assertIssueSearchProjection(
		t,
		store,
		issue.ID,
		"Export fails on iPad",
		"Export fails on iPad\n\nCustomer says it breaks after tapping share twice.",
		"export",
	)

	raw := "Export fails in Safari on iPad after tapping share twice."
	if err := store.UpdateIssueFields(ctx, issue.ID, IssueFieldUpdate{
		Raw:  &raw,
		Tags: []string{"export", "safari"},
		TagScores: []TagRelevance{
			{Tag: "export", Relevance: 0.95},
			{Tag: "safari", Relevance: 0.8},
		},
		Embedding: []float64{0.6, 0.4},
	}); err != nil {
		t.Fatalf("update issue fields: %v", err)
	}

	assertIssueSearchProjection(
		t,
		store,
		issue.ID,
		"Export fails in Safari on iPad after tapping share twice.",
		"Export fails on iPad\n\nCustomer says it breaks after tapping share twice.",
		"export safari",
	)
}

func assertIssueEmbeddingVectorText(t *testing.T, store *PostgresStore, id string, want string) {
	t.Helper()

	var got string
	if err := store.DB().QueryRowContext(
		context.Background(),
		`SELECT COALESCE(c.embedding_vector::text, i.embedding_vector::text, '')
		 FROM issues i
		 LEFT JOIN issue_content_projections c ON c.issue_id = i.id
		 WHERE i.id = $1`,
		id,
	).Scan(&got); err != nil {
		t.Fatalf("query embedding_vector for %s: %v", id, err)
	}
	if got != want {
		t.Fatalf("unexpected embedding_vector for %s: got %q want %q", id, got, want)
	}
}

func assertIssueSearchProjection(t *testing.T, store *PostgresStore, id, wantTitle, wantBody, wantTags string) {
	t.Helper()

	var gotTitle, gotBody, gotTags string
	if err := store.DB().QueryRowContext(
		context.Background(),
		`SELECT search_title, search_body, search_tags
		 FROM issue_content_projections
		 WHERE issue_id = $1`,
		id,
	).Scan(&gotTitle, &gotBody, &gotTags); err != nil {
		t.Fatalf("query issue search projection for %s: %v", id, err)
	}
	if gotTitle != wantTitle {
		t.Fatalf("unexpected search_title for %s: got %q want %q", id, gotTitle, wantTitle)
	}
	if gotBody != wantBody {
		t.Fatalf("unexpected search_body for %s: got %q want %q", id, gotBody, wantBody)
	}
	if gotTags != wantTags {
		t.Fatalf("unexpected search_tags for %s: got %q want %q", id, gotTags, wantTags)
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
	if tags[0].Name != "bug" { //nolint:goconst
		t.Fatalf("expected tags sorted by name, got %q first", tags[0].Name)
	}
	if len(tags[0].Embedding) != 2 {
		t.Fatalf("expected persisted tag embedding, got %#v", tags[0].Embedding)
	}
}

func TestPostgresStoreUpsertTagsSerializesConcurrentTagEvents(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	const workers = 8

	start := make(chan struct{})
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errCh <- store.UpsertTags(ctx, []Tag{
				{Name: "bug", Description: "software defect", Embedding: []float64{0.2, 0.8}},
			})
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent upsert tags: %v", err)
		}
	}

	var eventCount int
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM tag_events WHERE tag_name = $1`,
		"bug",
	).Scan(&eventCount); err != nil {
		t.Fatalf("count tag events: %v", err)
	}
	if eventCount != workers {
		t.Fatalf("expected %d tag events, got %d", workers, eventCount)
	}

	var projectionEventCount int
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT event_count FROM tag_projections WHERE name = $1`,
		"bug",
	).Scan(&projectionEventCount); err != nil {
		t.Fatalf("read tag projection event_count: %v", err)
	}
	if projectionEventCount != workers {
		t.Fatalf("expected projection event_count %d, got %d", workers, projectionEventCount)
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

func sparseUnitVector(dim, index int) []float64 { //nolint:unparam
	vector := make([]float64, dim)
	if index >= 0 && index < dim {
		vector[index] = 1
	}
	return vector
}

func TestPostgresStoreListTagsPreservesSpecificity(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	if err := store.UpsertTags(ctx, []Tag{
		{Name: "safari", Description: "webkit browser", Embedding: []float64{1, 0, 0}},
	}); err != nil {
		t.Fatalf("upsert tag: %v", err)
	}

	specificity := 0.85
	llm := 0.9
	emb := 0.73
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.UpdateTagSpecificity(ctx, "safari", &specificity, &llm, &emb, &now); err != nil {
		t.Fatalf("update tag specificity: %v", err)
	}

	tags, err := store.ListTags(ctx)
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	if len(tags) == 0 {
		t.Fatal("expected at least one tag")
	}

	tag := tags[0]
	if tag.Specificity == nil {
		t.Fatal("Specificity is nil, want non-nil")
	}
	if diff := *tag.Specificity - specificity; diff < -0.01 || diff > 0.01 {
		t.Errorf("Specificity: got %v, want ~%v", *tag.Specificity, specificity)
	}
	if tag.SpecificityLLM == nil {
		t.Fatal("SpecificityLLM is nil, want non-nil")
	}
	if diff := *tag.SpecificityLLM - llm; diff < -0.01 || diff > 0.01 {
		t.Errorf("SpecificityLLM: got %v, want ~%v", *tag.SpecificityLLM, llm)
	}
	if tag.SpecificityEmbedding == nil {
		t.Fatal("SpecificityEmbedding is nil, want non-nil")
	}
	if diff := *tag.SpecificityEmbedding - emb; diff < -0.01 || diff > 0.01 {
		t.Errorf("SpecificityEmbedding: got %v, want ~%v", *tag.SpecificityEmbedding, emb)
	}
	if tag.SpecificityComputedAt == nil || !tag.SpecificityComputedAt.Equal(now) {
		t.Errorf("SpecificityComputedAt: got %v, want %v", tag.SpecificityComputedAt, now)
	}
}

func TestPostgresStoreMergeTagsUsesAppendOnlyProjection(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	if err := store.UpsertTags(ctx, []Tag{
		{Name: "bug", Description: "software defect", Embedding: []float64{0.1, 0.9}},
		{Name: "defect", Description: "legacy alias", Embedding: []float64{0.2, 0.8}},
	}); err != nil {
		t.Fatalf("upsert tags: %v", err)
	}

	issue := BuildNewIssue("issue-tag-merge-append-only", CreateInput{
		Raw:       "alias tag should merge",
		CreatedBy: "Casey",
		Tags:      []string{"defect"},
		TagScores: []TagRelevance{{Tag: "defect", Relevance: 0.9}},
	})
	if err := store.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("save issue: %v", err)
	}

	if err := store.MergeTags(ctx, "bug", []string{"defect"}); err != nil {
		t.Fatalf("merge tags: %v", err)
	}

	tags, err := store.ListTags(ctx)
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "bug" {
		t.Fatalf("expected only canonical active tag, got %#v", tags)
	}

	var (
		status        string
		canonicalName string
		eventCount    int
	)
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT status, canonical_name, event_count
		 FROM tag_projections
		 WHERE name = $1`,
		"defect",
	).Scan(&status, &canonicalName, &eventCount); err != nil {
		t.Fatalf("read merged tag projection: %v", err)
	}
	if status != "merged" || canonicalName != "bug" || eventCount == 0 {
		t.Fatalf("unexpected merged tag projection status=%q canonical=%q eventCount=%d", status, canonicalName, eventCount)
	}

	var mergedEventCount int
	if err := store.DB().QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM tag_events WHERE tag_name = $1 AND kind = 'merged'`,
		"defect",
	).Scan(&mergedEventCount); err != nil {
		t.Fatalf("count merged tag events: %v", err)
	}
	if mergedEventCount != 1 {
		t.Fatalf("expected 1 merged tag event, got %d", mergedEventCount)
	}

	updated, err := store.Get(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get updated issue: %v", err)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "bug" {
		t.Fatalf("expected issue tags rewritten to canonical bug, got %#v", updated.Tags)
	}
	if len(updated.TagScores) != 1 || updated.TagScores[0].Tag != "bug" {
		t.Fatalf("expected issue tag scores rewritten to canonical bug, got %#v", updated.TagScores)
	}
}
