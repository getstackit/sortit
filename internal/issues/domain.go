package issues

// DisplayTags returns the display tags for an issue based on explicit tags or top tag scores.
func DisplayTags(explicitTags []string, scores []TagRelevance) []string {
	return displayTags(explicitTags, scores)
}

// DisplayTagsWithSpecificity returns display tags ranked by a blend of relevance
// and specificity (displayScore = relevance * 0.5 + specificity * 0.5).
func DisplayTagsWithSpecificity(explicitTags []string, scores []TagRelevance, tagSpecificity map[string]*float64) []string {
	return displayTagsWithSpecificity(explicitTags, scores, tagSpecificity)
}

// DefaultActor returns the actor name, falling back to "You" if empty.
func DefaultActor(value string) string {
	return defaultActor(value)
}

// InitialDiscussion returns the initial discussion post for an issue.
func InitialDiscussion(issue Issue) []IssuePost {
	return initialDiscussion(issue)
}

// IssuePostID returns the post ID for a given issue and sequence number.
func IssuePostID(issueID string, sequence int) string {
	return issuePostID(issueID, sequence)
}

// NormalizeIssueLinkType validates and normalizes a link type string.
func NormalizeIssueLinkType(value IssueLinkType) IssueLinkType {
	return normalizeIssueLinkType(value)
}

// CloseIssuePost returns the activity post body for closing an issue.
func CloseIssuePost(closedBy, reason, reasonNote string) string {
	return closeIssuePost(closedBy, reason, reasonNote)
}

// ReopenIssuePost returns the activity post body for reopening an issue.
func ReopenIssuePost() string {
	return reopenIssuePost()
}

// AssignIssuePost returns the activity post body for assigning an issue.
func AssignIssuePost(assignee string) string {
	return assignIssuePost(assignee)
}

// IssueReference creates an IssueReference snapshot from an Issue.
func IssueReferenceFrom(issue Issue) IssueReference {
	return issueReference(issue)
}

// SanitizeIssueIDs deduplicates and trims a list of issue IDs.
func SanitizeIssueIDs(ids []string) []string {
	return sanitizeIssueIDs(ids)
}

// HydrateOperation populates participant issue references in an operation.
func HydrateOperation(operation IssueOperation, issuesByID map[string]Issue) IssueOperation {
	return hydrateOperation(operation, issuesByID)
}

// MapProjectionIssuesFromIssues converts full issues into the minimal payload
// needed to rebuild the map projection.
func MapProjectionIssuesFromIssues(items []Issue) []MapProjectionIssue {
	projectionItems := make([]MapProjectionIssue, len(items))
	for i, item := range items {
		projectionItems[i] = MapProjectionIssue{
			ID:         item.ID,
			Raw:        item.Raw,
			Tags:       append([]string(nil), item.Tags...),
			Status:     normalizeIssueStatus(item.Status),
			AssignedTo: item.AssignedTo,
			TagScores:  copyTagScores(item.TagScores),
			Embedding:  copyEmbedding(item.Embedding),
			Links:      cloneIssueLinks(item.Links),
		}
	}
	return projectionItems
}
