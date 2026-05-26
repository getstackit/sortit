package regions

import (
	"sort"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

// Age bucket labels in display order.
const (
	AgeBucketUnderOneWeek     = "<1w"
	AgeBucketOneToFourWeeks   = "1-4w"
	AgeBucketOneToThreeMonths = "1-3m"
	AgeBucketThreeMonthsPlus  = "3m+"
)

// RegionWithMetrics is the per-region payload returned by the metrics API.
type RegionWithMetrics struct {
	Region  domain.Region        `json:"region"`
	Metrics domain.RegionMetrics `json:"metrics"`
}

// ComputeMass returns the total, open, and closed issue counts for the
// region. Counts are derived from BelongsTo, so the membership semantics
// stay in one place.
func ComputeMass(items []issues.Issue, key domain.RegionKey) (mass, open, closed int) {
	for _, issue := range items {
		if !BelongsTo(issue, key) {
			continue
		}
		mass++
		if issue.Status == issues.StatusClosed {
			closed++
		} else {
			open++
		}
	}
	return mass, open, closed
}

// ComputeAgeBuckets counts currently-open issues in the region by age
// (now - CreatedAt) using the fixed bucket layout: <1w, 1-4w, 1-3m, 3m+.
// Boundaries are half-open: an issue exactly at the 7-day mark falls into
// the 1-4w bucket.
func ComputeAgeBuckets(items []issues.Issue, key domain.RegionKey, now time.Time) []domain.AgeBucket {
	buckets := []domain.AgeBucket{
		{Label: AgeBucketUnderOneWeek},
		{Label: AgeBucketOneToFourWeeks},
		{Label: AgeBucketOneToThreeMonths},
		{Label: AgeBucketThreeMonthsPlus},
	}
	for _, issue := range items {
		if issue.Status != issues.StatusOpen {
			continue
		}
		if !BelongsTo(issue, key) {
			continue
		}
		age := now.Sub(issue.CreatedAt)
		switch {
		case age < 7*24*time.Hour:
			buckets[0].Count++
		case age < 28*24*time.Hour:
			buckets[1].Count++
		case age < 90*24*time.Hour:
			buckets[2].Count++
		default:
			buckets[3].Count++
		}
	}
	return buckets
}

// ComputeMetrics produces phase-1 metrics for a single region. Returns
// false if no issues are members (zero mass).
func ComputeMetrics(items []issues.Issue, key domain.RegionKey, window domain.TimeWindow, now time.Time) (domain.RegionMetrics, bool) {
	mass, open, closed := ComputeMass(items, key)
	if mass == 0 {
		return domain.RegionMetrics{}, false
	}
	return domain.RegionMetrics{
		Key:        key,
		Window:     window,
		Mass:       mass,
		MassOpen:   open,
		MassClosed: closed,
		AgeBuckets: ComputeAgeBuckets(items, key, now),
	}, true
}

// ListRegionsWithMetrics enumerates every tag-region with at least one
// issue at or above MembershipFloor and computes phase-1 metrics for each.
// Results are sorted by Mass descending, ties broken by tag name.
func ListRegionsWithMetrics(items []issues.Issue, window domain.TimeWindow, now time.Time) []RegionWithMetrics {
	tags := presentTags(items)
	out := make([]RegionWithMetrics, 0, len(tags))
	for _, tag := range tags {
		key := domain.RegionKey{Kind: domain.RegionKindTag, ID: tag}
		metrics, ok := ComputeMetrics(items, key, window, now)
		if !ok {
			continue
		}
		out = append(out, RegionWithMetrics{
			Region:  domain.Region{Key: key, Label: tag},
			Metrics: metrics,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Metrics.Mass != out[j].Metrics.Mass {
			return out[i].Metrics.Mass > out[j].Metrics.Mass
		}
		return out[i].Region.Key.ID < out[j].Region.Key.ID
	})
	return out
}

// presentTags returns the sorted set of normalized tag names that appear
// at relevance >= MembershipFloor on at least one issue.
func presentTags(items []issues.Issue) []string {
	seen := make(map[string]struct{})
	for _, issue := range items {
		for _, score := range issue.TagScores {
			if score.Relevance < MembershipFloor {
				continue
			}
			seen[domain.NormalizeTagName(score.Tag)] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		if tag == "" {
			continue
		}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}
