package regions

import (
	"slices"
	"sort"
	"strings"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

// MatchesPredicate reports whether the issue satisfies the predicate.
// Used for custom region membership.
//
// Semantics:
//   - Tags: at least one TagPredicate must match (OR within the list).
//     A TagPredicate matches when the issue has a TagRelevance whose
//     normalized name equals predicate.Tag and Relevance >= MinRelevance.
//   - Statuses: the issue's Status must be in the list (when non-empty).
//   - Authors: the issue's CreatedBy must be in the list (when non-empty).
//   - MaxAgeDays: (now - CreatedAt).Hours / 24 must be <= the value
//     (when non-nil).
//
// All populated dimensions are combined with AND.
func MatchesPredicate(issue issues.Issue, predicate domain.CustomRegionPredicate, now time.Time) bool {
	if len(predicate.Tags) > 0 {
		if !matchesAnyTagPredicate(issue, predicate.Tags) {
			return false
		}
	}
	if len(predicate.Statuses) > 0 {
		if !slices.Contains(predicate.Statuses, string(issue.Status)) {
			return false
		}
	}
	if len(predicate.Authors) > 0 {
		author := strings.TrimSpace(issue.CreatedBy)
		if !slices.ContainsFunc(predicate.Authors, func(a string) bool {
			return strings.EqualFold(strings.TrimSpace(a), author)
		}) {
			return false
		}
	}
	if predicate.MaxAgeDays != nil {
		ageDays := now.Sub(issue.CreatedAt).Hours() / 24
		if ageDays > float64(*predicate.MaxAgeDays) {
			return false
		}
	}
	return true
}

func matchesAnyTagPredicate(issue issues.Issue, predicates []domain.TagPredicate) bool {
	for _, predicate := range predicates {
		target := domain.NormalizeTagName(predicate.Tag)
		if target == "" {
			continue
		}
		for _, score := range issue.TagScores {
			if domain.NormalizeTagName(score.Tag) != target {
				continue
			}
			if score.Relevance >= predicate.MinRelevance {
				return true
			}
		}
	}
	return false
}

// ListCustomRegionsWithMetrics applies each definition to the issue
// corpus, computes the standard region metrics, and returns the result
// sorted by mass descending.
func ListCustomRegionsWithMetrics(
	regions []domain.CustomRegion,
	items []issues.Issue,
	events []issues.Event,
	window domain.TimeWindow,
	now time.Time,
) []RegionWithMetrics {
	if len(regions) == 0 {
		return nil
	}
	out := make([]RegionWithMetrics, 0, len(regions))
	for _, region := range regions {
		members := make([]issues.Issue, 0, len(items)/4)
		for _, issue := range items {
			if !MatchesPredicate(issue, region.Definition, now) {
				continue
			}
			members = append(members, issue)
		}
		key := domain.RegionKey{Kind: domain.RegionKindCustom, ID: region.ID}
		metrics, ok := ComputeMetricsForMembers(key, members, events, window, now)
		if !ok {
			// Even zero-mass regions are useful to show the user; surface
			// them with empty metrics so the UI can render "0 members."
			metrics = domain.RegionMetrics{Key: key, Window: window}
		}
		out = append(out, RegionWithMetrics{
			Region: domain.Region{
				Key:         key,
				Label:       region.Label,
				Description: region.Description,
			},
			Metrics: metrics,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Metrics.Mass != out[j].Metrics.Mass {
			return out[i].Metrics.Mass > out[j].Metrics.Mass
		}
		return out[i].Region.Label < out[j].Region.Label
	})
	return out
}
