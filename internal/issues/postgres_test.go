package issues

import (
	"context"
	"sync"
	"testing"

	"splat/internal/testpostgres"
)

var issuesPostgresHarness struct {
	once    sync.Once
	harness *testpostgres.Harness
	err     error
}

func TestPostgresStoreCreateListAndGet(t *testing.T) {
	store := newPostgresTestStore(t)

	created, err := store.Create(context.Background(), CreateInput{
		Raw:       "  add postgres storage  ",
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
	if created.Raw != "add postgres storage" {
		t.Fatalf("expected trimmed raw value, got %q", created.Raw)
	}
	if created.CreatedBy != "Casey" {
		t.Fatalf("expected trimmed creator, got %q", created.CreatedBy)
	}
	if created.Status != StatusOpen {
		t.Fatalf("expected created issue to be open, got %q", created.Status)
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
	if len(loaded.Discussion) != 1 {
		t.Fatalf("expected initial discussion post, got %#v", loaded.Discussion)
	}
	if loaded.Discussion[0].Raw != created.Raw {
		t.Fatalf("expected discussion to preserve original raw, got %q", loaded.Discussion[0].Raw)
	}
}

func TestPostgresStoreRefineAppendsDiscussionAndUpdatesCanonicalIssue(t *testing.T) {
	store := newPostgresTestStore(t)

	created, err := store.Create(context.Background(), CreateInput{
		Raw:       "export fails on ipad",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "export", Relevance: 0.8}},
		Embedding: []float64{0.25, 0.5},
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	refined, err := store.Refine(context.Background(), created.ID, RefineInput{
		PostRaw:      "Customer says this only happens in Safari after tapping share twice.",
		CanonicalRaw: "Export fails in Safari on iPad after tapping share twice.",
		CreatedBy:    "Jordan",
		TagScores: []TagRelevance{
			{Tag: "export", Relevance: 0.95},
			{Tag: "safari", Relevance: 0.73},
		},
		Embedding: []float64{0.7, 0.2},
	})
	if err != nil {
		t.Fatalf("refine issue: %v", err)
	}

	if refined.Raw != "Export fails in Safari on iPad after tapping share twice." {
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

	loaded, err := store.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get refined issue: %v", err)
	}
	if loaded.Raw != refined.Raw {
		t.Fatalf("expected persisted canonical raw %q, got %q", refined.Raw, loaded.Raw)
	}
	if len(loaded.Discussion) != 2 {
		t.Fatalf("expected persisted discussion history, got %#v", loaded.Discussion)
	}
}

func TestPostgresStoreCloseAndReopenIssue(t *testing.T) {
	store := newPostgresTestStore(t)

	created, err := store.Create(context.Background(), CreateInput{
		Raw: "close me",
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	closed, err := store.CloseIssue(context.Background(), created.ID, "Casey")
	if err != nil {
		t.Fatalf("close issue: %v", err)
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

	reopened, err := store.ReopenIssue(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("reopen issue: %v", err)
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

func TestPostgresStorePersistsIssueRelationshipsAndOperationHistory(t *testing.T) {
	store := newPostgresTestStore(t)

	parent, err := store.Create(context.Background(), CreateInput{
		Raw:       "Large onboarding redesign",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "feature", Relevance: 0.9}},
		Embedding: []float64{0.1, 0.2},
	})
	if err != nil {
		t.Fatalf("create parent issue: %v", err)
	}

	splitResult, err := store.SplitIssue(context.Background(), SplitInput{
		SourceID: parent.ID,
		Children: []SplitChildInput{
			{Raw: "Add onboarding checklist", TagScores: []TagRelevance{{Tag: "feature", Relevance: 0.8}}},
			{Raw: "Improve invite acceptance copy", TagScores: []TagRelevance{{Tag: "ux", Relevance: 0.7}}},
		},
		CreatedBy:   "Casey",
		Note:        "Break the umbrella issue into shippable work.",
		CloseSource: true,
	})
	if err != nil {
		t.Fatalf("split issue: %v", err)
	}
	if splitResult.Operation.Kind != IssueOperationKindSplit {
		t.Fatalf("expected split operation, got %q", splitResult.Operation.Kind)
	}
	if len(splitResult.CreatedIssues) != 2 {
		t.Fatalf("expected 2 child issues, got %d", len(splitResult.CreatedIssues))
	}

	linkedResult, err := store.LinkIssues(context.Background(), LinkInput{
		SourceID:  splitResult.CreatedIssues[0].ID,
		TargetID:  splitResult.CreatedIssues[1].ID,
		Type:      IssueLinkTypeRelatedTo,
		CreatedBy: "Jordan",
		Note:      "These two child issues ship together.",
	})
	if err != nil {
		t.Fatalf("link issues: %v", err)
	}
	if linkedResult.Operation.Kind != IssueOperationKindLink {
		t.Fatalf("expected link operation, got %q", linkedResult.Operation.Kind)
	}

	combinedResult, err := store.CombineIssues(context.Background(), CombineInput{
		SourceIDs: []string{splitResult.CreatedIssues[0].ID, splitResult.CreatedIssues[1].ID},
		Raw:       "Deliver a tighter onboarding flow with checklist and invite improvements.",
		CreatedBy: "Taylor",
		Note:      "Roll the child issues into a single delivery artifact.",
		TagScores: []TagRelevance{{Tag: "feature", Relevance: 0.95}},
		Embedding: []float64{0.3, 0.7},
	})
	if err != nil {
		t.Fatalf("combine issues: %v", err)
	}
	if len(combinedResult.CreatedIssues) != 1 {
		t.Fatalf("expected one combined issue, got %d", len(combinedResult.CreatedIssues))
	}

	loadedParent, err := store.Get(context.Background(), parent.ID)
	if err != nil {
		t.Fatalf("get parent issue: %v", err)
	}
	if loadedParent.Status != StatusClosed {
		t.Fatalf("expected parent to be closed after split, got %q", loadedParent.Status)
	}
	if len(loadedParent.Links) != 4 {
		t.Fatalf("expected parent to expose split relationships, got %#v", loadedParent.Links)
	}
	if len(loadedParent.Operations) != 1 {
		t.Fatalf("expected parent to show one split operation, got %#v", loadedParent.Operations)
	}

	loadedChild, err := store.Get(context.Background(), splitResult.CreatedIssues[0].ID)
	if err != nil {
		t.Fatalf("get child issue: %v", err)
	}
	if loadedChild.Status != StatusClosed {
		t.Fatalf("expected child to be closed after combine, got %q", loadedChild.Status)
	}
	if len(loadedChild.Operations) != 3 {
		t.Fatalf("expected child to show split, link, and combine operations, got %#v", loadedChild.Operations)
	}
	if loadedChild.Links[0].RelatedIssue == nil {
		t.Fatalf("expected hydrated related issue reference, got %#v", loadedChild.Links[0])
	}

	loadedCombined, err := store.Get(context.Background(), combinedResult.CreatedIssues[0].ID)
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

func TestPostgresStoreReplaceResetsSequenceFromLoadedItems(t *testing.T) {
	store := newPostgresTestStore(t)

	if err := store.Replace(context.Background(), FixtureIssues()); err != nil {
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
