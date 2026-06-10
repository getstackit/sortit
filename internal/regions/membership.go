// Package regions defines the unit of analysis for Tier 1 cartography:
// a tag, cluster, or custom slice of the issue corpus together with the
// metrics that summarize it. Phase 1 implements only tag-regions; cluster
// and custom kinds are reserved in the domain types and rejected at the
// API boundary.
package regions

import (
	"sortit/internal/domain"
	"sortit/internal/issues"
)

// MembershipFloor is the relevance threshold at which an issue is
// considered a member of a tag-region. The value matches the verifier's
// keep threshold so region membership stays consistent with the rest of
// the system's notion of "this tag actually applies."
const MembershipFloor = 0.4

// BelongsTo reports whether the issue is a member of the region. For
// tag-regions, membership requires a TagRelevance whose Tag matches the
// region ID and whose Relevance is at least MembershipFloor. The presence
// of a Negation does not by itself exclude membership: relevance and
// negation are independent signals.
//
// The counterargument to ignoring Negation: an issue with Relevance 0.5
// and Negation 0.7 has net signed relevance −0.2 — the system's strongest
// evidence says the tag does NOT apply, yet the issue still counts as a
// region member. Once the math layer consumes signed scores (r⁺ − r⁻),
// membership should likely switch to net relevance
// (Relevance − Negation ≥ MembershipFloor). Kept as-is for now so region
// membership changes land together with the signed-score consumers
// rather than ahead of them. See docs/scoring-search-map.md.
//
// Cluster and custom region kinds are not implemented in phase 1 and
// always return false.
func BelongsTo(issue issues.Issue, key domain.RegionKey) bool {
	if key.Kind != domain.RegionKindTag {
		return false
	}
	tag := domain.NormalizeTagName(key.ID)
	if tag == "" {
		return false
	}
	for _, score := range issue.TagScores {
		if domain.NormalizeTagName(score.Tag) != tag {
			continue
		}
		return score.Relevance >= MembershipFloor
	}
	return false
}
