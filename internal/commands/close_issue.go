package commands

import (
	"context"
	"strings"
	"time"

	"splat/internal/issues"
)

type CloseIssue struct {
	ID       string
	ClosedBy string
}

type CloseIssueHandler struct {
	Store  issues.Store
	Events issues.EventStore
}

func (h CloseIssueHandler) Handle(ctx context.Context, input CloseIssue) (issues.Issue, error) {
	id := strings.TrimSpace(input.ID)
	issue, err := h.Store.Get(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}

	if issue.Status == issues.StatusClosed && issue.ClosedAt != nil {
		return issue, nil
	}

	closedAt := time.Now().UTC()
	actor := issues.DefaultActor(input.ClosedBy)
	status := issues.StatusClosed

	if err := h.Store.UpdateIssueFields(ctx, id, issues.IssueFieldUpdate{
		Status:   &status,
		ClosedAt: &closedAt,
		ClosedBy: &actor,
	}); err != nil {
		return issues.Issue{}, err
	}

	post := issues.NewDiscussionPost(id, issue.Discussion, issues.CloseIssuePost(actor), actor, "closed")
	if err := h.Store.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}

	if h.Events != nil {
		_ = h.Events.RecordEvent(ctx, issues.Event{
			ID:        post.ID,
			Kind:      "closed",
			IssueID:   id,
			CreatedBy: actor,
			CreatedAt: closedAt,
			Body:      issues.CloseIssuePost(actor),
		})
	}

	return h.Store.Get(ctx, id)
}
