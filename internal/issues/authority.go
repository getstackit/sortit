package issues

import "splat/internal/scoring"

// IssueAuthority computes how canonical an issue is based on inbound
// duplicate_of and merged_into links. The score is bounded between 0 and 1,
// with 4+ inbound canonical links yielding 1.0. This measures whether an issue
// acts as the authoritative landing point for a problem, not raw graph
// connectivity (which is hubness, a separate signal).
func IssueAuthority(issue Issue) float64 {
	count := 0
	for _, link := range issue.Links {
		if link.TargetIssueID != issue.ID {
			continue
		}
		if link.Type == IssueLinkTypeDuplicateOf || link.Type == IssueLinkTypeMergedInto {
			count++
		}
	}
	return CanonicalLinkAuthority(count)
}

// CanonicalLinkAuthority computes authority from a count of inbound
// duplicate_of and merged_into links.
func CanonicalLinkAuthority(count int) float64 {
	a := float64(count) * scoring.AuthorityPerLink
	if a > 1 {
		return 1
	}
	return a
}
