package commands

import (
	"context"

	"splat/internal/issues"
)

type ProgressIssue struct {
	ID        string
	Raw       string
	CreatedBy string
}

type ProgressIssueHandler struct {
	Runner *CommandRunner
	Events issues.EventPublisher
}

func (h ProgressIssueHandler) Handle(ctx context.Context, input ProgressIssue) (progressed issues.Issue, err error) {
	id, err := issues.ValidateID(input.ID)
	if err != nil {
		return issues.Issue{}, err
	}
	raw, err := issues.ValidateRaw(input.Raw, "raw")
	if err != nil {
		return issues.Issue{}, err
	}

	uow, finish, err := Begin(ctx, h.Runner)
	if err != nil {
		return issues.Issue{}, err
	}

	var progressEvent issues.Event
	defer func() {
		FinishAndPublish(&err, finish, ctx, h.Events, progressEvent)
	}()

	current, err := uow.Get(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}
	if err := issues.EnsureMutable(current); err != nil {
		return issues.Issue{}, err
	}

	post := issues.NewDiscussionPost(id, current.Discussion, raw, input.CreatedBy, "progress")
	if err := uow.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}

	progressEvent = issues.Event{
		ID:        post.ID,
		Kind:      "progress",
		IssueID:   id,
		CreatedBy: post.CreatedBy,
		CreatedAt: post.CreatedAt,
		Body:      raw,
	}
	_ = uow.RecordEvent(ctx, progressEvent)

	return uow.Get(ctx, id)
}
