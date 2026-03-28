package issueanalytics

import (
	"splat/internal/issues"
	"splat/internal/scoring"
)

// IssueHubness computes a graph-connectedness score for an issue based on its
// link count. The score is bounded between 0 and 1, with 7+ links yielding 1.0.
// This measures how much an issue acts as an anchor in the issue graph, not
// whether it is the canonical representative (which is authority, a separate signal).
func IssueHubness(issue issues.Issue) float64 {
	return LinkCountHubness(len(issue.Links))
}

// MapProjectionIssueHubness computes hubness for a MapProjectionIssue.
func MapProjectionIssueHubness(issue issues.MapProjectionIssue) float64 {
	return LinkCountHubness(len(issue.Links))
}

// LinkCountHubness computes hubness from a raw link count.
func LinkCountHubness(linkCount int) float64 {
	h := float64(linkCount) * scoring.HubnessPerLink
	if h > 1 {
		return 1
	}
	return h
}
