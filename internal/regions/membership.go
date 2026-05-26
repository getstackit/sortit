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
