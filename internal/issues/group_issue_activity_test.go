package issues

import (
	"slices"
	"testing"

	"sortit/internal/issues/issuesdb"
)

func TestNormalizeIssueIDs(t *testing.T) {
	got := normalizeIssueIDs([]string{" a ", "", "b", "a", "  ", "b"})
	if want := []string{"a", "b"}; !slices.Equal(got, want) {
		t.Fatalf("normalizeIssueIDs = %v, want %v", got, want)
	}
}

func TestGroupIssueActivityBucketsPostsAndLinks(t *testing.T) {
	ids := []string{"issue-1", "issue-2"}
	posts := []issuesdb.IssuePost{
		{ID: "p1", IssueID: "issue-1", Kind: "refinement", CreatedAtUnixNano: 100},
		{ID: "p2", IssueID: "issue-2", Kind: "progress", CreatedAtUnixNano: 200},
		{ID: "p3", IssueID: "issue-unknown", Kind: "refinement", CreatedAtUnixNano: 300}, // not requested → ignored
	}
	links := []issuesdb.IssueLink{
		{ID: "l1", SourceIssueID: "issue-1", TargetIssueID: "issue-2", Type: "relates_to", CreatedAtUnixNano: 50},       // both endpoints requested
		{ID: "l2", SourceIssueID: "issue-1", TargetIssueID: "issue-outside", Type: "relates_to", CreatedAtUnixNano: 60}, // only source requested
		{ID: "l3", SourceIssueID: "issue-2", TargetIssueID: "issue-2", Type: "relates_to", CreatedAtUnixNano: 70},       // self-link → attach once
	}

	got := groupIssueActivity(ids, posts, links)

	if len(got) != 2 {
		t.Fatalf("expected entries for exactly the 2 requested ids, got %d (%v)", len(got), got)
	}

	a1 := got["issue-1"]
	if gotIDs := postIDs(a1.Posts); !slices.Equal(gotIDs, []string{"p1"}) {
		t.Fatalf("issue-1 posts = %v, want [p1]", gotIDs)
	}
	// issue-1 is an endpoint of l1 (source) and l2 (source).
	if gotIDs := linkIDs(a1.Links); !slices.Equal(gotIDs, []string{"l1", "l2"}) {
		t.Fatalf("issue-1 links = %v, want [l1 l2]", gotIDs)
	}

	a2 := got["issue-2"]
	if gotIDs := postIDs(a2.Posts); !slices.Equal(gotIDs, []string{"p2"}) {
		t.Fatalf("issue-2 posts = %v, want [p2]", gotIDs)
	}
	// issue-2 is an endpoint of l1 (target) and l3 (self-link, counted once).
	if gotIDs := linkIDs(a2.Links); !slices.Equal(gotIDs, []string{"l1", "l3"}) {
		t.Fatalf("issue-2 links = %v, want [l1 l3]", gotIDs)
	}
}

func TestGroupIssueActivityIncludesRequestedIDsWithNoActivity(t *testing.T) {
	got := groupIssueActivity([]string{"issue-x"}, nil, nil)
	a, ok := got["issue-x"]
	if !ok {
		t.Fatalf("expected requested id present even with no activity")
	}
	if len(a.Posts) != 0 || len(a.Links) != 0 {
		t.Fatalf("expected empty activity, got %+v", a)
	}
}

func postIDs(posts []IssuePost) []string {
	ids := make([]string, 0, len(posts))
	for _, p := range posts {
		ids = append(ids, p.ID)
	}
	return ids
}

func linkIDs(links []IssueLink) []string {
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.ID)
	}
	return ids
}
