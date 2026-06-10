// Package tagcooccurrence computes pairwise tag co-occurrence statistics
// across a corpus of issues. The output powers the structural
// anti-correlation signal that math-evolution.md §4.1 calls out as a third
// source of r_i⁻ once the corpus is large enough to be meaningful.
//
// This signal is consumed on the hot path: internal/search feeds the top
// anti-correlated pairs of a query's target tag into the live search
// ranking as a penalty (internal/map candidateAntiCorrelationPenalty,
// weighted by scoring.RegionAntiCorrelationPenalty). It also backs the
// debug diagnostics endpoint.
//
// Because it shapes user-facing rankings, ImplicitNegative is statistically
// guarded: a pair only emits a non-zero score when the conditioning tag's
// presence count and the pair's expected joint count under independence
// clear the floors in internal/scoring (AntiCorrelationMinPresence,
// AntiCorrelationMinExpectedJoint). Above the floors the score is a shrunk
// lift, 1 − (joint + s) / (presenceCount × baseProb + s) with smoothing
// s = scoring.AntiCorrelationSmoothing, clamped to [0, 1] — so small
// corpora and base-rate-dominated pairs read as zero rather than as
// spurious anti-correlation.
package tagcooccurrence

import (
	"cmp"
	"slices"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/scoring"
)

// defaultRelevanceFloor mirrors the enrichment-side floor: a tag is
// considered "present" on an issue when its Relevance meets this threshold.
// Synthetic Relevance:0 rows (from analyzer negations) are intentionally
// excluded because they represent absence, not presence.
const defaultRelevanceFloor = 0.08

// PairStats summarizes a directed pair (i → j). The implicit-negative signal
// reads as: "given tag i is present, how much less likely is tag j than the
// base rate?" — a positive number indicates structural anti-correlation.
type PairStats struct {
	Tag             string  `json:"tag"`
	JointCount      int     `json:"jointCount"`
	ConditionalProb float64 `json:"conditionalProb"`
	BaseProb        float64 `json:"baseProb"`
	// ImplicitNegative is a shrunk lift score in [0, 1]:
	// 1 − (joint + s) / (count(i) × baseProb(j) + s), with smoothing
	// s = scoring.AntiCorrelationSmoothing. It is forced to zero unless
	// count(i) ≥ scoring.AntiCorrelationMinPresence and the expected joint
	// count count(i) × baseProb(j) ≥ scoring.AntiCorrelationMinExpectedJoint,
	// so sparse corpora produce no anti-correlation signal.
	ImplicitNegative float64 `json:"implicitNegative"`
}

// TagStats holds aggregate counts plus a sorted list of pair statistics
// against every other tag in the corpus.
type TagStats struct {
	Tag   string      `json:"tag"`
	Count int         `json:"count"`
	Pairs []PairStats `json:"pairs"`
}

// Stats is the corpus-wide co-occurrence projection. Tags are sorted by
// descending count so consumers can quickly drop tags below a sample-size
// floor.
type Stats struct {
	IssueCount int        `json:"issueCount"`
	TagCount   int        `json:"tagCount"`
	Tags       []TagStats `json:"tags"`
}

// Compute walks the corpus once, counts tag presence per issue, then derives
// the pairwise statistics. Tags that never reach the relevance floor are
// dropped entirely — they have no positive evidence and contribute no
// useful co-occurrence signal.
func Compute(items []issues.Issue) Stats {
	return ComputeWithFloor(items, defaultRelevanceFloor)
}

// ComputeWithFloor mirrors Compute but lets callers override the relevance
// floor. Useful for tests and for experimenting with stricter thresholds.
func ComputeWithFloor(items []issues.Issue, relevanceFloor float64) Stats {
	if len(items) == 0 {
		return Stats{}
	}

	presence := make(map[string]int)
	pair := make(map[string]map[string]int)

	for _, item := range items {
		seen := make(map[string]struct{}, len(item.TagScores))
		for _, score := range item.TagScores {
			if score.Relevance < relevanceFloor {
				continue
			}
			name := domain.NormalizeTagName(score.Tag)
			if name == "" {
				continue
			}
			seen[name] = struct{}{}
		}
		if len(seen) == 0 {
			continue
		}
		issueTags := make([]string, 0, len(seen))
		for name := range seen {
			presence[name]++
			issueTags = append(issueTags, name)
		}
		for _, a := range issueTags {
			row := pair[a]
			if row == nil {
				row = make(map[string]int)
				pair[a] = row
			}
			for _, b := range issueTags {
				if a == b {
					continue
				}
				row[b]++
			}
		}
	}

	n := float64(len(items))
	tags := make([]TagStats, 0, len(presence))
	for name, count := range presence {
		// Iterate over every other tag in the corpus, not just tags this one
		// co-occurred with — the anti-correlation signal lives precisely in
		// pairs that don't co-occur (joint = 0).
		pairs := make([]PairStats, 0, len(presence)-1)
		for other, otherCount := range presence {
			if other == name {
				continue
			}
			joint := pair[name][other]
			baseProb := float64(otherCount) / n
			conditional := 0.0
			if count > 0 {
				conditional = float64(joint) / float64(count)
			}
			implicit := implicitNegative(count, joint, baseProb)
			pairs = append(pairs, PairStats{
				Tag:              other,
				JointCount:       joint,
				ConditionalProb:  round2(conditional),
				BaseProb:         round2(baseProb),
				ImplicitNegative: round2(implicit),
			})
		}
		slices.SortStableFunc(pairs, func(a, b PairStats) int {
			if c := cmp.Compare(b.ImplicitNegative, a.ImplicitNegative); c != 0 {
				return c
			}
			return cmp.Compare(a.Tag, b.Tag)
		})
		tags = append(tags, TagStats{
			Tag:   name,
			Count: count,
			Pairs: pairs,
		})
	}

	slices.SortStableFunc(tags, func(a, b TagStats) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})

	return Stats{
		IssueCount: len(items),
		TagCount:   len(tags),
		Tags:       tags,
	}
}

// LookupTag returns the per-tag stats and true when the tag appears in the
// corpus at or above the relevance floor used when Stats was computed.
func (s Stats) LookupTag(name string) (TagStats, bool) {
	normalized := domain.NormalizeTagName(name)
	for _, t := range s.Tags {
		if t.Tag == normalized {
			return t, true
		}
	}
	return TagStats{}, false
}

// implicitNegative scores how anti-correlated tag j is with conditioning
// tag i, given count(i), the observed joint count, and baseProb(j). It
// returns the shrunk lift 1 − (joint + s) / (expected + s) clamped to
// [0, 1], where expected = count(i) × baseProb(j) is the joint count
// independence predicts. The scoring floors gate the signal entirely:
// below them, an observed joint of zero is indistinguishable from a
// corpus too small to have produced a co-occurrence yet.
func implicitNegative(presenceCount, jointCount int, baseProb float64) float64 {
	if presenceCount < scoring.AntiCorrelationMinPresence {
		return 0
	}
	expected := float64(presenceCount) * baseProb
	if expected < scoring.AntiCorrelationMinExpectedJoint {
		return 0
	}
	s := scoring.AntiCorrelationSmoothing
	lift := 1 - (float64(jointCount)+s)/(expected+s)
	return min(max(lift, 0), 1)
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
