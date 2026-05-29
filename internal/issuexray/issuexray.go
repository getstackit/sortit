// Package issuexray computes the Tier 2 per-issue scorecard documented
// in docs/planning.md. Each axis is a small, interpretable scalar
// derived from data already collected by the enrichment pipeline; the
// scorecard surfaces them in one place on the issue detail page.
package issuexray

import (
	"math"
	"slices"
	"strings"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/tagcooccurrence"
)

// StrongRelevanceFloor mirrors regions.MembershipFloor — tags below this
// are excluded from the coherence and definition computations.
const StrongRelevanceFloor = 0.4

// IssueXRay is the per-issue Tier 2 scorecard.
type IssueXRay struct {
	Specificity   *float64           `json:"specificity,omitempty"`
	Coherence     *float64           `json:"coherence,omitempty"`
	Definition    *float64           `json:"definition,omitempty"`
	Connectedness ConnectednessScore `json:"connectedness"`
	Lifecycle     LifecyclePosition  `json:"lifecycle"`
}

// ConnectednessScore aggregates inbound/outbound graph activity.
type ConnectednessScore struct {
	Links      int     `json:"links"`
	Operations int     `json:"operations"`
	Score      float64 `json:"score"`
}

// LifecyclePosition categorizes the issue into early/mid/late based on
// refinement count and age. The thresholds are intentionally coarse;
// the scorecard's job is interpretability, not precision.
type LifecyclePosition struct {
	Label       string `json:"label"`
	Refinements int    `json:"refinements"`
	AgeDays     int    `json:"ageDays"`
}

// Compute returns the scorecard. tagSpecificity is a map from
// normalized tag name to specificity score; coOccurrence may be nil
// when the projection is unavailable (degrades to nil Coherence).
func Compute(
	issue issues.Issue,
	tagSpecificity map[string]float64,
	coOccurrence *tagcooccurrence.Stats,
	now time.Time,
) IssueXRay {
	out := IssueXRay{}
	strongTags := strongTagsOf(issue)

	out.Specificity = computeSpecificity(issue, tagSpecificity)
	out.Coherence = computeCoherence(strongTags, coOccurrence)
	out.Definition = computeDefinition(issue)
	out.Connectedness = computeConnectedness(issue)
	out.Lifecycle = computeLifecycle(issue, now)
	return out
}

// strongTagsOf returns the normalized names of tags whose relevance
// reaches StrongRelevanceFloor.
func strongTagsOf(issue issues.Issue) []string {
	out := make([]string, 0, len(issue.TagScores))
	for _, score := range issue.TagScores {
		if score.Relevance < StrongRelevanceFloor {
			continue
		}
		name := domain.NormalizeTagName(score.Tag)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

// computeSpecificity is the relevance-weighted mean of the catalog's
// per-tag specificity scores over the issue's tag set. Returns nil when
// no tag has a specificity score available.
func computeSpecificity(issue issues.Issue, tagSpecificity map[string]float64) *float64 {
	if len(tagSpecificity) == 0 || len(issue.TagScores) == 0 {
		return nil
	}
	weightedSum := 0.0
	weight := 0.0
	for _, score := range issue.TagScores {
		if score.Relevance <= 0 {
			continue
		}
		name := domain.NormalizeTagName(score.Tag)
		spec, ok := tagSpecificity[name]
		if !ok {
			continue
		}
		weightedSum += score.Relevance * spec
		weight += score.Relevance
	}
	if weight == 0 {
		return nil
	}
	v := weightedSum / weight
	v = math.Round(v*1000) / 1000
	return &v
}

// computeCoherence returns the mean P(j|i) across all ordered strong-tag
// pairs in the issue. High values mean the tags really go together;
// low values mean the issue's tag set is internally unusual. Returns
// nil when fewer than two strong tags or when the projection isn't
// available.
func computeCoherence(strong []string, stats *tagcooccurrence.Stats) *float64 {
	if len(strong) < 2 || stats == nil || stats.IssueCount == 0 {
		return nil
	}
	sum := 0.0
	pairs := 0
	for _, a := range strong {
		tagStats, ok := stats.LookupTag(a)
		if !ok {
			continue
		}
		for _, pair := range tagStats.Pairs {
			if !contains(strong, pair.Tag) {
				continue
			}
			sum += pair.ConditionalProb
			pairs++
		}
	}
	if pairs == 0 {
		return nil
	}
	v := sum / float64(pairs)
	v = math.Round(v*1000) / 1000
	return &v
}

// computeDefinition measures how cleanly the issue belongs to one
// region. We use the ratio of the strongest tag's relevance to the
// total relevance mass — 1.0 means one dominant tag (well-defined);
// near 1/N means the issue's relevance is spread evenly (drifty).
// Returns nil when no tag exceeds the floor.
func computeDefinition(issue issues.Issue) *float64 {
	top := 0.0
	total := 0.0
	for _, score := range issue.TagScores {
		if score.Relevance <= 0 {
			continue
		}
		if score.Relevance > top {
			top = score.Relevance
		}
		total += score.Relevance
	}
	if top < StrongRelevanceFloor || total == 0 {
		return nil
	}
	v := top / total
	v = math.Round(v*1000) / 1000
	return &v
}

// computeConnectedness counts graph activity. Score saturates around
// 6 links + operations.
func computeConnectedness(issue issues.Issue) ConnectednessScore {
	links := len(issue.Links)
	ops := len(issue.Operations)
	combined := float64(links + ops)
	score := math.Min(combined/6.0, 1.0)
	score = math.Round(score*100) / 100
	return ConnectednessScore{
		Links:      links,
		Operations: ops,
		Score:      score,
	}
}

// computeLifecycle classifies the issue into early/mid/late based on
// refinement count + age. Thresholds chosen for interpretability:
//   - early: < 7d and < 2 refinements
//   - late:  >= 30d and >= 4 refinements
//   - mid:   everything else
func computeLifecycle(issue issues.Issue, now time.Time) LifecyclePosition {
	refinements := 0
	if issue.LifecycleMetrics != nil {
		refinements = issue.LifecycleMetrics.RefinementCount
	}
	ageDays := 0
	if !issue.CreatedAt.IsZero() {
		ageDays = int(now.Sub(issue.CreatedAt).Hours() / 24)
	}

	label := "mid"
	switch {
	case ageDays < 7 && refinements < 2:
		label = "early"
	case ageDays >= 30 && refinements >= 4:
		label = "late"
	}
	return LifecyclePosition{
		Label:       label,
		Refinements: refinements,
		AgeDays:     ageDays,
	}
}

func contains(items []string, name string) bool {
	return slices.Contains(items, strings.TrimSpace(name))
}
