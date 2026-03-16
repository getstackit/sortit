package commands

import (
	"context"
	"strings"
	"time"

	"splat/internal/issues"
)

type CloseIssue struct {
	ID         string
	ClosedBy   string
	Reason     string
	ReasonNote string
}

type CloseIssueHandler struct {
	Runner *CommandRunner
	Events issues.EventPublisher
}

func (h CloseIssueHandler) Handle(ctx context.Context, input CloseIssue) (closed issues.Issue, err error) {
	id := strings.TrimSpace(input.ID)

	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "fixed"
	}
	if err := issues.ValidateCloseReason(reason); err != nil {
		return issues.Issue{}, err
	}
	reasonNote := strings.TrimSpace(input.ReasonNote)

	uow, finish, err := Begin(ctx, h.Runner)
	if err != nil {
		return issues.Issue{}, err
	}

	var closedEvent issues.Event
	defer func() {
		FinishAndPublish(&err, finish, ctx, h.Events, closedEvent)
	}()

	issue, err := uow.Get(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}

	if issue.Status == issues.StatusClosed && issue.ClosedAt != nil {
		return issue, nil
	}

	closedAt := time.Now().UTC()
	actor := issues.DefaultActor(input.ClosedBy)
	status := issues.StatusClosed

	if err := uow.UpdateIssueFields(ctx, id, issues.IssueFieldUpdate{
		Status:           &status,
		ClosedAt:         &closedAt,
		ClosedBy:         &actor,
		ClosedReason:     &reason,
		ClosedReasonNote: &reasonNote,
	}); err != nil {
		return issues.Issue{}, err
	}

	postBody := issues.CloseIssuePost(actor, reason, reasonNote)
	post := issues.NewDiscussionPost(id, issue.Discussion, postBody, actor, "closed")
	if err := uow.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}

	closedEvent = issues.Event{
		ID:        post.ID,
		Kind:      "closed",
		IssueID:   id,
		CreatedBy: actor,
		CreatedAt: closedAt,
		Body:      postBody,
	}
	_ = uow.RecordEvent(ctx, closedEvent)

	return uow.Get(ctx, id)
}
