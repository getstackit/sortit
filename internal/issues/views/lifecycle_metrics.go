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

	hydrated := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		hydrated = append(hydrated, HydrateIssueWithVelocity(ctx, reader, item, now))
	}
	return hydrated
}
