package issues

import (
	"fmt"
	"strings"
)

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
