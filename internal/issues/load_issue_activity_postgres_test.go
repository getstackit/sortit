package issues

import (
	"context"
	"slices"
	"testing"
	"time"
)

// TestPostgresStoreLoadIssueActivityMatchesPerIssueDetail proves the batched
// velocity load returns exactly the posts and links the per-issue GetIssueDetail
// path would, so swapping search/explore hydration to the batch path leaves the
// computed velocity unchanged.
func TestPostgresStoreLoadIssueActivityMatchesPerIssueDetail(t *testing.T) {
	store := newPostgresTestStore(t)
	ctx := context.Background()

	alpha := BuildNewIssue(NewIssueID(), CreateInput{Raw: "alpha issue", CreatedBy: "Casey", Embedding: []float64{1, 0}})
	beta := BuildNewIssue(NewIssueID(), CreateInput{Raw: "beta issue", CreatedBy: "Jordan", Embedding: []float64{0, 1}})
	quiet := BuildNewIssue(NewIssueID(), CreateInput{Raw: "quiet issue", CreatedBy: "Sam", Embedding: []float64{1, 1}})
	unrequested := BuildNewIssue(NewIssueID(), CreateInput{Raw: "unrequested issue", CreatedBy: "Lee", Embedding: []float64{0.5, 0.5}})
	for _, issue := range []Issue{alpha, beta, quiet, unrequested} {
		if err := store.SaveIssue(ctx, issue); err != nil {
			t.Fatalf("save issue %s: %v", issue.ID, err)
		}
	}

	// A refinement on alpha and a progress update on beta give them real velocity
	// events; quiet keeps only its initial report post.
	loadedAlpha, err := store.Get(ctx, alpha.ID)
	if err != nil {
		t.Fatalf("get alpha: %v", err)
	}
	if err := store.SaveIssuePost(ctx, NewDiscussionPost(alpha.ID, loadedAlpha.Discussion, "more detail on alpha", "Jordan", "refinement")); err != nil {
		t.Fatalf("save alpha post: %v", err)
	}
	loadedBeta, err := store.Get(ctx, beta.ID)
	if err != nil {
		t.Fatalf("get beta: %v", err)
	}
	if err := store.SaveIssuePost(ctx, NewDiscussionPost(beta.ID, loadedBeta.Discussion, "progress on beta", "Casey", "progress")); err != nil {
		t.Fatalf("save beta post: %v", err)
	}

	// A single link between beta (source) and alpha (target) must surface in BOTH
	// issues' activity, matching ListIssueLinksForIssue(source=id, target=id).
	if err := store.SaveLink(ctx, IssueLink{
		ID:            NewOperationID(),
		Type:          IssueLinkTypeDerivedFrom,
		SourceIssueID: beta.ID,
		TargetIssueID: alpha.ID,
		CreatedBy:     "Jordan",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save link: %v", err)
	}

	requested := []string{alpha.ID, beta.ID, quiet.ID}
	got, err := store.LoadIssueActivity(ctx, requested)
	if err != nil {
		t.Fatalf("load issue activity: %v", err)
	}

	if len(got) != len(requested) {
		t.Fatalf("expected activity entries for %d requested ids, got %d", len(requested), len(got))
	}
	if _, present := got[unrequested.ID]; present {
		t.Fatalf("unrequested issue must not appear in activity map")
	}

	// The link is shared by alpha and beta.
	if !slices.Contains(linkIDs(got[alpha.ID].Links), got[beta.ID].Links[0].ID) {
		t.Fatalf("expected the shared link to appear in both alpha and beta activity")
	}

	// Equivalence with the per-issue detail path for every requested issue.
	for _, id := range requested {
		detail, err := store.GetIssueDetail(ctx, id)
		if err != nil {
			t.Fatalf("get issue detail %s: %v", id, err)
		}
		if got, want := postIDs(got[id].Posts), postIDs(detail.Discussion); !slices.Equal(got, want) {
			t.Fatalf("issue %s posts = %v, want (from detail) %v", id, got, want)
		}
		if got, want := linkIDs(got[id].Links), linkIDs(detail.Links); !slices.Equal(got, want) {
			t.Fatalf("issue %s links = %v, want (from detail) %v", id, got, want)
		}
	}
}
