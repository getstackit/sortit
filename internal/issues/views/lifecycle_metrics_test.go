package issueviews

import (
	"context"
	"errors"
	"testing"
	"time"

	"sortit/internal/issues"
)

// fakeActivityReader implements both issues.IssueActivityReader and
// issues.IssueDetailReader so a single fake can exercise the batch path and the
// per-issue fallback, and assert which one ran.
type fakeActivityReader struct {
	activity    map[string]issues.IssueActivity
	loadErr     error
	loadCalls   int
	loadIDs     []string
	detailCalls int
}

func (f *fakeActivityReader) LoadIssueActivity(_ context.Context, ids []string) (map[string]issues.IssueActivity, error) {
	f.loadCalls++
	f.loadIDs = append(f.loadIDs, ids...)
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.activity, nil
}

func (f *fakeActivityReader) GetIssueDetail(_ context.Context, id string) (issues.Issue, error) {
	f.detailCalls++
	a := f.activity[id]
	return issues.Issue{ID: id, Discussion: a.Posts, Links: a.Links}, nil
}

// detailOnlyReader implements only issues.IssueDetailReader, forcing the
// per-issue path.
type detailOnlyReader struct {
	detail      map[string]issues.Issue
	detailCalls int
}

func (f *detailOnlyReader) GetIssueDetail(_ context.Context, id string) (issues.Issue, error) {
	f.detailCalls++
	return f.detail[id], nil
}

func recentRefinement(now time.Time) issues.IssuePost {
	return issues.IssuePost{CreatedAt: now.Add(-24 * time.Hour), Sequence: 2, Kind: "refinement"}
}

func TestHydrateIssuesWithVelocityUsesBatchPath(t *testing.T) {
	now := time.Now().UTC()
	reader := &fakeActivityReader{
		activity: map[string]issues.IssueActivity{
			"issue-1": {Posts: []issues.IssuePost{recentRefinement(now)}},
			"issue-2": {}, // no activity
		},
	}
	items := []issues.Issue{{ID: "issue-1"}, {ID: "issue-2"}}

	got := HydrateIssuesWithVelocity(context.Background(), reader, items, now)

	if reader.loadCalls != 1 {
		t.Fatalf("expected exactly 1 batched activity load, got %d", reader.loadCalls)
	}
	if reader.detailCalls != 0 {
		t.Fatalf("expected 0 per-issue GetIssueDetail calls on the batch path, got %d", reader.detailCalls)
	}
	if len(reader.loadIDs) != 2 {
		t.Fatalf("expected both ids passed to the batch load, got %v", reader.loadIDs)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 hydrated issues, got %d", len(got))
	}

	if v := velocityOf(t, got[0]); v <= 0 {
		t.Fatalf("expected issue-1 velocity > 0, got %v", v)
	}
	if len(got[0].Discussion) != 1 {
		t.Fatalf("expected issue-1 Discussion populated from activity, got %d posts", len(got[0].Discussion))
	}
	if v := velocityOf(t, got[1]); v != 0 {
		t.Fatalf("expected issue-2 (no activity) velocity 0, got %v", v)
	}
}

func TestHydrateIssuesWithVelocityFallsBackWhenNoBatchSupport(t *testing.T) {
	now := time.Now().UTC()
	reader := &detailOnlyReader{detail: map[string]issues.Issue{
		"issue-1": {ID: "issue-1", Discussion: []issues.IssuePost{recentRefinement(now)}},
	}}
	items := []issues.Issue{{ID: "issue-1"}}

	got := HydrateIssuesWithVelocity(context.Background(), reader, items, now)

	if reader.detailCalls != 1 {
		t.Fatalf("expected per-issue fallback (1 GetIssueDetail call), got %d", reader.detailCalls)
	}
	if v := velocityOf(t, got[0]); v <= 0 {
		t.Fatalf("expected velocity attached via fallback, got %v", v)
	}
}

func TestHydrateIssuesWithVelocityFallsBackOnBatchError(t *testing.T) {
	now := time.Now().UTC()
	reader := &fakeActivityReader{
		loadErr: errors.New("boom"),
		activity: map[string]issues.IssueActivity{
			"issue-1": {Posts: []issues.IssuePost{recentRefinement(now)}},
		},
	}
	items := []issues.Issue{{ID: "issue-1"}}

	got := HydrateIssuesWithVelocity(context.Background(), reader, items, now)

	if reader.loadCalls != 1 {
		t.Fatalf("expected 1 batch attempt before fallback, got %d", reader.loadCalls)
	}
	if reader.detailCalls != 1 {
		t.Fatalf("expected per-issue fallback after batch error (1 GetIssueDetail call), got %d", reader.detailCalls)
	}
	if v := velocityOf(t, got[0]); v <= 0 {
		t.Fatalf("expected velocity attached via fallback, got %v", v)
	}
}

func velocityOf(t *testing.T, issue issues.Issue) float64 {
	t.Helper()
	if issue.LifecycleMetrics == nil || issue.LifecycleMetrics.Velocity == nil {
		t.Fatalf("issue %s has no velocity attached: %+v", issue.ID, issue.LifecycleMetrics)
	}
	return *issue.LifecycleMetrics.Velocity
}
