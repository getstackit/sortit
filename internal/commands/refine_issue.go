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
	Store    issues.Store
	Enricher *services.IssueEnricher
	Events   issues.EventStore
}

func (h RefineIssueHandler) Handle(ctx context.Context, input RefineIssue) (issues.Issue, error) {
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

	post := issues.NewDiscussionPost(current.ID, current.Discussion, postRaw, enriched.CreatedBy, "refinement")
	if err := h.Store.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}

	if err := h.Store.UpdateIssueFields(ctx, current.ID, issues.IssueFieldUpdate{
		Raw:       &canonicalRaw,
		Tags:      issues.DisplayTags(enriched.Tags, enriched.TagScores),
		TagScores: enriched.TagScores,
		Embedding: enriched.Embedding,
	}); err != nil {
		return issues.Issue{}, err
	}

	if h.Events != nil {
		_ = h.Events.RecordEvent(ctx, issues.Event{
			ID:        post.ID,
			Kind:      "refinement",
			IssueID:   current.ID,
			CreatedBy: post.CreatedBy,
			CreatedAt: post.CreatedAt,
			Body:      postRaw,
		})
	}

	return h.Store.Get(ctx, current.ID)
}
