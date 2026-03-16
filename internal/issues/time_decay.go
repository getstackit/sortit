package issues

import (
	"math"
	"time"
)

const (
	issueFreshnessFloor        = 0.3
	issueFreshnessHalfLifeDays = 90.0
)

func IssueFreshnessWeight(issue Issue, now time.Time) float64 {
	return TimeDecayWeight(issueFreshnessTimestamp(issue), now, issueFreshnessFloor, issueFreshnessHalfLifeDays)
}

func TimeDecayWeight(timestamp, now time.Time, floor, halfLifeDays float64) float64 {
	floor = clamp01(floor)
	if halfLifeDays <= 0 {
		halfLifeDays = issueFreshnessHalfLifeDays
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if timestamp.IsZero() {
		return 1
	}

	age := now.Sub(timestamp)
	if age < 0 {
		age = 0
	}
	ageDays := age.Hours() / 24
	decay := math.Exp(-math.Ln2 * ageDays / halfLifeDays)
	return floor + (1-floor)*decay
}

func issueFreshnessTimestamp(issue Issue) time.Time {
	latest := issue.CreatedAt
	if issue.ClosedAt != nil && issue.ClosedAt.After(latest) {
		latest = issue.ClosedAt.UTC()
	}
	for _, post := range issue.Discussion {
		if post.CreatedAt.After(latest) {
			latest = post.CreatedAt.UTC()
		}
	}
	for _, link := range issue.Links {
		if link.CreatedAt.After(latest) {
			latest = link.CreatedAt.UTC()
		}
	}
	return latest
}
