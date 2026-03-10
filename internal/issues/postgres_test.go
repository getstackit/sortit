package issues

import (
	"context"
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

	id, err := store.NextIssueID(context.Background())
	if err != nil {
		t.Fatalf("next issue id: %v", err)
	}

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

	if issue.ID != "issue-000001" {
		t.Fatalf("expected first generated ID, got %q", issue.ID)
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
}

func TestPostgresStoreRefineAppendsDiscussionAndUpdatesCanonicalIssue(t *testing.T) {
	store := newPostgresTestStore(t)

	id, err := store.NextIssueID(context.Background())
	if err != nil {
		t.Fatalf("next issue id: %v", err)
	}

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
}

func TestPostgresStoreCloseAndReopenIssue(t *testing.T) {
	store := newPostgresTestStore(t)

	id, err := store.NextIssueID(context.Background())
	if err != nil {
		t.Fatalf("next issue id: %v", err)
	}

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

func TestPostgresStorePersistsIssueRelationshipsAndOperationHistory(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	// Create parent issue
	parentID, err := store.NextIssueID(ctx)
	if err != nil {
		t.Fatalf("next issue id: %v", err)
	}
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
	child1ID, err := store.NextIssueID(ctx)
	if err != nil {
		t.Fatalf("next issue id: %v", err)
	}
	child1 := BuildNewIssue(child1ID, CreateInput{
		Raw:       "Add onboarding checklist",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "feature", Relevance: 0.8}},
	})
	if err := store.SaveIssue(ctx, child1); err != nil {
		t.Fatalf("save child1: %v", err)
	}

	child2ID, err := store.NextIssueID(ctx)
	if err != nil {
		t.Fatalf("next issue id: %v", err)
	}
	child2 := BuildNewIssue(child2ID, CreateInput{
		Raw:       "Improve invite acceptance copy",
		CreatedBy: "Casey",
		TagScores: []TagRelevance{{Tag: "ux", Relevance: 0.7}},
	})
	if err := store.SaveIssue(ctx, child2); err != nil {
		t.Fatalf("save child2: %v", err)
	}

	// Create split operation
	splitOpID, err := store.NextOperationID(ctx)
	if err != nil {
		t.Fatalf("next op id: %v", err)
	}
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
	linkOpID, err := store.NextOperationID(ctx)
	if err != nil {
		t.Fatalf("next op id: %v", err)
	}
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
	combinedID, err := store.NextIssueID(ctx)
	if err != nil {
		t.Fatalf("next issue id: %v", err)
	}
	combined := BuildNewIssue(combinedID, CreateInput{
		Raw:       "Deliver a tighter onboarding flow with checklist and invite improvements.",
		CreatedBy: "Taylor",
		TagScores: []TagRelevance{{Tag: "feature", Relevance: 0.95}},
		Embedding: []float64{0.3, 0.7},
	})
	if err := store.SaveIssue(ctx, combined); err != nil {
		t.Fatalf("save combined issue: %v", err)
	}

	combineOpID, err := store.NextOperationID(ctx)
	if err != nil {
		t.Fatalf("next op id: %v", err)
	}
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

func TestPostgresStoreReplaceResetsSequenceFromLoadedItems(t *testing.T) {
	store := newPostgresTestStore(t)

	if err := store.Replace(context.Background(), FixtureIssues()); err != nil {
		t.Fatalf("replace issues with seeds: %v", err)
	}

	id, err := store.NextIssueID(context.Background())
	if err != nil {
		t.Fatalf("next issue id: %v", err)
	}
	issue := BuildNewIssue(id, CreateInput{Raw: "created after sample load"})
	if err := store.SaveIssue(context.Background(), issue); err != nil {
		t.Fatalf("save issue after replace: %v", err)
	}
	if issue.ID != "issue-000007" {
		t.Fatalf("expected sequence to continue after seeded items, got %q", issue.ID)
	}

	if err := store.Replace(context.Background(), nil); err != nil {
		t.Fatalf("clear store: %v", err)
	}

	resetID, err := store.NextIssueID(context.Background())
	if err != nil {
		t.Fatalf("next issue id: %v", err)
	}
	resetIssue := BuildNewIssue(resetID, CreateInput{Raw: "created after reset"})
	if err := store.SaveIssue(context.Background(), resetIssue); err != nil {
		t.Fatalf("save issue after reset: %v", err)
	}
	if resetIssue.ID != "issue-000001" {
		t.Fatalf("expected sequence reset after clearing store, got %q", resetIssue.ID)
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
