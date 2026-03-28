package issueanalytics

import (
	"math"
	"time"

	"splat/internal/issues"
)

const (
	issueVelocityWindowDays   = 30.0
	issueVelocityHalfLifeDays = 14.0
	issueVelocitySaturation   = 2.5
	refinementVelocityWeight  = 1.0
	progressVelocityWeight    = 0.85
	linkVelocityWeight        = 0.65
)

type issueVelocityEvent struct {
	createdAt time.Time
	weight    float64
}

func AttachIssueVelocity(posts []issues.IssuePost, links []issues.IssueLink, metrics *issues.IssueLifecycleMetrics, now time.Time) *issues.IssueLifecycleMetrics {
	velocity, recentActivityCount := ComputeIssueVelocity(posts, links, now)
	if metrics == nil {
		metrics = &issues.IssueLifecycleMetrics{}
	}
	metrics.Velocity = cloneFloat64Ptr(&velocity)
	metrics.RecentActivityCount = recentActivityCount
	return metrics
}

func ComputeIssueVelocity(posts []issues.IssuePost, links []issues.IssueLink, now time.Time) (float64, int) {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	cutoff := now.Add(-time.Duration(issueVelocityWindowDays*24) * time.Hour)
	events := issueVelocityEvents(posts, links)

	var (
		weighted float64
		count    int
	)
	for _, event := range events {
		createdAt := event.createdAt.UTC()
		if createdAt.IsZero() || createdAt.After(now) || createdAt.Before(cutoff) {
			continue
		}
		ageDays := now.Sub(createdAt).Hours() / 24
		decay := math.Exp(-math.Ln2 * ageDays / issueVelocityHalfLifeDays)
		weighted += event.weight * decay
		count++
	}

	if count == 0 {
		return 0, 0
	}

	return clamp01(1 - math.Exp(-weighted/issueVelocitySaturation)), count
}

func issueVelocityEvents(posts []issues.IssuePost, links []issues.IssueLink) []issueVelocityEvent {
	events := make([]issueVelocityEvent, 0, len(posts)+len(links))
	for _, post := range posts {
		switch issuePostKind(post) {
		case "refinement":
			events = append(events, issueVelocityEvent{createdAt: post.CreatedAt, weight: refinementVelocityWeight})
		case "progress":
			events = append(events, issueVelocityEvent{createdAt: post.CreatedAt, weight: progressVelocityWeight})
		}
	}
	for _, link := range links {
		events = append(events, issueVelocityEvent{createdAt: link.CreatedAt, weight: linkVelocityWeight})
	}
	return events
}
