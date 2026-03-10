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
	Store  issues.Store
	Events issues.EventStore
}

func (h ProgressIssueHandler) Handle(ctx context.Context, input ProgressIssue) (issues.Issue, error) {
	id, err := issues.ValidateID(input.ID)
	if err != nil {
		return issues.Issue{}, err
	}
	raw, err := issues.ValidateRaw(input.Raw, "raw")
	if err != nil {
		return issues.Issue{}, err
	}

	current, err := h.Store.Get(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}
	if err := issues.EnsureMutable(current); err != nil {
		return issues.Issue{}, err
	}

	post := issues.NewDiscussionPost(id, current.Discussion, raw, input.CreatedBy, "progress")
	if err := h.Store.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}

	if h.Events != nil {
		_ = h.Events.RecordEvent(ctx, issues.Event{
			ID:        post.ID,
			Kind:      "progress",
			IssueID:   id,
			CreatedBy: post.CreatedBy,
			CreatedAt: post.CreatedAt,
			Body:      raw,
		})
	}

	return h.Store.Get(ctx, id)
}
