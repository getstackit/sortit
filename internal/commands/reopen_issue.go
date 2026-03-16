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
	Runner *CommandRunner
	Events issues.EventPublisher
}

func (h ReopenIssueHandler) Handle(ctx context.Context, input ReopenIssue) (reopened issues.Issue, err error) {
	id := strings.TrimSpace(input.ID)

	uow, finish, err := Begin(ctx, h.Runner)
	if err != nil {
		return issues.Issue{}, err
	}

	var reopenedEvent issues.Event
	defer func() {
		FinishAndPublish(&err, finish, ctx, h.Events, reopenedEvent)
	}()

	issue, err := uow.Get(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}

	if issue.Status == issues.StatusOpen {
		return issue, nil
	}

	reopenedAt := time.Now().UTC()
	status := issues.StatusOpen
	if err := uow.UpdateIssueFields(ctx, id, issues.IssueFieldUpdate{
		LifecycleCreatedAt: &reopenedAt,
		Status:             &status,
	}); err != nil {
		return issues.Issue{}, err
	}

	post := issues.NewDiscussionPost(id, issue.Discussion, issues.ReopenIssuePost(), "", "reopened")
	if err := uow.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}

	reopenedEvent = issues.Event{
		ID:        post.ID,
		Kind:      "reopened",
		IssueID:   id,
		CreatedBy: post.CreatedBy,
		CreatedAt: post.CreatedAt,
		Body:      issues.ReopenIssuePost(),
	}
	_ = uow.RecordEvent(ctx, reopenedEvent)

	return uow.Get(ctx, id)
}
