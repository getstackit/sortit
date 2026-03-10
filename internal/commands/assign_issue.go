package commands

import (
	"context"
	"strings"
	"time"

	"splat/internal/issues"
)

type AssignIssue struct {
	ID         string
	AssignedTo string
}

type AssignIssueHandler struct {
	Store  issues.Store
	Events issues.EventStore
}

func (h AssignIssueHandler) Handle(ctx context.Context, input AssignIssue) (issues.Issue, error) {
	id := strings.TrimSpace(input.ID)
	assignee := strings.TrimSpace(input.AssignedTo)

	issue, err := h.Store.Get(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}

	if err := h.Store.UpdateIssueFields(ctx, id, issues.IssueFieldUpdate{
		AssignedTo: &assignee,
	}); err != nil {
		return issues.Issue{}, err
	}

	assignedAt := time.Now().UTC()
	post := issues.NewDiscussionPost(id, issue.Discussion, issues.AssignIssuePost(assignee), "", "assigned")
	if err := h.Store.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}

	if h.Events != nil {
		_ = h.Events.RecordEvent(ctx, issues.Event{
			ID:        post.ID,
			Kind:      "assigned",
			IssueID:   id,
			CreatedBy: post.CreatedBy,
			CreatedAt: assignedAt,
			Body:      issues.AssignIssuePost(assignee),
		})
	}

	return h.Store.Get(ctx, id)
}
