package commands

import (
	"context"

	"splat/internal/issues"
	"splat/internal/services"
)

type RefineIssue struct {
	ID        string
	Raw       string
	CreatedBy string
}

type RefineIssueHandler struct {
	Runner   *CommandRunner
	Store    issues.Store
	Enricher *services.IssueEnricher
	Events   issues.EventPublisher
}

func (h RefineIssueHandler) Handle(ctx context.Context, input RefineIssue) (refined issues.Issue, err error) {
	current, err := h.Store.Get(ctx, input.ID)
	if err != nil {
		return issues.Issue{}, err
	}

	if err := issues.EnsureMutable(current); err != nil {
		return issues.Issue{}, err
	}

	enriched, err := h.Enricher.AnalyzeRefineInput(ctx, current, input.Raw, input.CreatedBy)
	if err != nil {
		return issues.Issue{}, err
	}

	postRaw, canonicalRaw, err := issues.ValidateRefineInput(enriched)
	if err != nil {
		return issues.Issue{}, err
	}

	uow, finish, err := Begin(ctx, h.Runner)
	if err != nil {
		return issues.Issue{}, err
	}

	var refinementEvent issues.Event
	defer func() {
		FinishAndPublish(&err, finish, ctx, h.Events, refinementEvent)
	}()

	current, err = uow.Get(ctx, input.ID)
	if err != nil {
		return issues.Issue{}, err
	}
	if err := issues.EnsureMutable(current); err != nil {
		return issues.Issue{}, err
	}

	post := issues.NewDiscussionPost(current.ID, current.Discussion, postRaw, enriched.CreatedBy, "refinement")
	if err := uow.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}

	if err := uow.UpdateIssueFields(ctx, current.ID, issues.IssueFieldUpdate{
		Raw:       &canonicalRaw,
		Tags:      issues.DisplayTags(enriched.Tags, enriched.TagScores),
		TagScores: enriched.TagScores,
		Embedding: enriched.Embedding,
	}); err != nil {
		return issues.Issue{}, err
	}

	refinementEvent = issues.Event{
		ID:        post.ID,
		Kind:      "refinement",
		IssueID:   current.ID,
		CreatedBy: post.CreatedBy,
		CreatedAt: post.CreatedAt,
		Body:      postRaw,
	}
	_ = uow.RecordEvent(ctx, refinementEvent)

	return uow.Get(ctx, current.ID)
}
