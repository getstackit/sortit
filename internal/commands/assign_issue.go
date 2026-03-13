package commands

import (
	"context"
	"strings"

	"splat/internal/issues"
)

type AssignIssue struct {
	ID         string
	AssignedTo string
	CreatedBy  string
}

type AssignIssueHandler struct {
	Runner *CommandRunner
	Events issues.EventPublisher
}

func (h AssignIssueHandler) Handle(ctx context.Context, input AssignIssue) (assigned issues.Issue, err error) {
	id := strings.TrimSpace(input.ID)
	assignee := strings.TrimSpace(input.AssignedTo)

	uow, finish, err := Begin(ctx, h.Runner)
	if err != nil {
		return issues.Issue{}, err
	}

	var assignedEvent issues.Event
	defer func() {
		FinishAndPublish(&err, finish, ctx, h.Events, assignedEvent)
	}()

	issue, err := uow.Get(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}

	if err := uow.UpdateIssueFields(ctx, id, issues.IssueFieldUpdate{
		AssignedTo: &assignee,
	}); err != nil {
		return issues.Issue{}, err
	}

	post := issues.NewDiscussionPost(
		id,
		issue.Discussion,
		issues.AssignIssuePost(assignee),
		issues.DefaultActor(input.CreatedBy),
		"assigned",
	)
	if err := uow.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}

	assignedEvent = issues.Event{
		ID:        post.ID,
		Kind:      "assigned",
		IssueID:   id,
		CreatedBy: post.CreatedBy,
		CreatedAt: post.CreatedAt,
		Body:      issues.AssignIssuePost(assignee),
	}
	_ = uow.RecordEvent(ctx, assignedEvent)

	return uow.Get(ctx, id)
}
