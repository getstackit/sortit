package commands

import (
	"context"
	"strings"
	"time"

	"splat/internal/issues"
)

type ReopenIssue struct {
	ID string
}

type ReopenIssueHandler struct {
	Store  issues.Store
	Events issues.EventStore
}

func (h ReopenIssueHandler) Handle(ctx context.Context, input ReopenIssue) (issues.Issue, error) {
	id := strings.TrimSpace(input.ID)
	issue, err := h.Store.Get(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}

	if issue.Status == issues.StatusOpen {
		return issue, nil
	}

	status := issues.StatusOpen
	if err := h.Store.UpdateIssueFields(ctx, id, issues.IssueFieldUpdate{
		Status: &status,
	}); err != nil {
		return issues.Issue{}, err
	}

	reopenedAt := time.Now().UTC()
	post := issues.NewDiscussionPost(id, issue.Discussion, issues.ReopenIssuePost(), "", "reopened")
	if err := h.Store.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}

	if h.Events != nil {
		_ = h.Events.RecordEvent(ctx, issues.Event{
			ID:        post.ID,
			Kind:      "reopened",
			IssueID:   id,
			CreatedBy: post.CreatedBy,
			CreatedAt: reopenedAt,
			Body:      issues.ReopenIssuePost(),
		})
	}

	return h.Store.Get(ctx, id)
}
