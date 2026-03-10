package issues

import (
	"fmt"
	"strings"
	"time"
)

func appendActivityPost(posts []IssuePost, issueID string, createdAt time.Time, createdBy string, kind string, raw string) []IssuePost {
	next := cloneIssuePosts(posts)
	next = append(next, IssuePost{
		ID:        issuePostID(issueID, len(next)+1),
		IssueID:   issueID,
		Raw:       strings.TrimSpace(raw),
		CreatedBy: defaultActor(createdBy),
		CreatedAt: createdAt.UTC(),
		Sequence:  len(next) + 1,
		Kind:      strings.TrimSpace(kind),
	})
	return next
}

func closeIssuePost(createdBy string) string {
	actor := defaultActor(createdBy)
	return fmt.Sprintf("Closed by %s.", actor)
}

func reopenIssuePost() string {
	return "Reopened issue."
}

func assignIssuePost(assignee string) string {
	assignee = strings.TrimSpace(assignee)
	if assignee == "" {
		return "Unassigned issue."
	}
	return fmt.Sprintf("Assigned to %s.", assignee)
}
