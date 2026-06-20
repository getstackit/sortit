package issueviews

import (
	"context"
	"time"

	"sortit/internal/issueanalytics"
	"sortit/internal/issues"
)

func HydrateIssueWithVelocity(
	ctx context.Context,
	reader issues.IssueDetailReader,
	item issues.Issue,
	now time.Time,
) issues.Issue {
	if reader != nil {
		detailed, err := reader.GetIssueDetail(ctx, item.ID)
		if err == nil {
			item = detailed
		}
	}
	item.LifecycleMetrics = issueanalytics.AttachIssueVelocity(item.Discussion, item.Links, item.LifecycleMetrics, now)
	return item
}

func HydrateIssuesWithVelocity(
	ctx context.Context,
	reader issues.IssueDetailReader,
	items []issues.Issue,
	now time.Time,
) []issues.Issue {
	if len(items) == 0 {
		return nil
	}

	// Preferred path: batch-load the velocity inputs (posts + links) for every
	// issue in a fixed number of queries instead of calling GetIssueDetail per
	// issue (which fans out to 5-6 queries each). Falls back to the per-issue
	// path when the reader can't batch or the batch load fails.
	if batch, ok := reader.(issues.IssueActivityReader); ok {
		if hydrated, ok := hydrateVelocityBatched(ctx, batch, items, now); ok {
			return hydrated
		}
	}

	hydrated := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		hydrated = append(hydrated, HydrateIssueWithVelocity(ctx, reader, item, now))
	}
	return hydrated
}

// hydrateVelocityBatched attaches velocity to every item using a single batched
// activity load. It populates Discussion and Links to match the per-issue path
// (explore consumes Links downstream), but deliberately does not fetch the rest
// of issue detail — ranking and the search/explore responses only read the
// computed velocity, not operations, references, or enrichment.
func hydrateVelocityBatched(
	ctx context.Context,
	reader issues.IssueActivityReader,
	items []issues.Issue,
	now time.Time,
) ([]issues.Issue, bool) {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}

	activity, err := reader.LoadIssueActivity(ctx, ids)
	if err != nil {
		return nil, false
	}

	hydrated := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		a := activity[item.ID]
		item.Discussion = a.Posts
		item.Links = a.Links
		item.LifecycleMetrics = issueanalytics.AttachIssueVelocity(a.Posts, a.Links, item.LifecycleMetrics, now)
		hydrated = append(hydrated, item)
	}
	return hydrated, true
}
