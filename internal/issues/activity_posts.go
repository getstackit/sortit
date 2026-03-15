package issues

import (
	"fmt"
	"strings"
)

func closeIssuePost(createdBy, reason, reasonNote string) string {
	actor := defaultActor(createdBy)
	if reason == "" {
		return fmt.Sprintf("Closed by %s.", actor)
	}
	if reasonNote == "" {
		return fmt.Sprintf("%s closed this as %s.", actor, reason)
	}
	return fmt.Sprintf("%s closed this as %s — %s", actor, reason, reasonNote)
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
