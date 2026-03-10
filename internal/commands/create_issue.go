package commands

import (
	"context"

	"splat/internal/issues"
	"splat/internal/services"
)

type CreateIssue struct {
	Raw       string
	Tags      []string
	CreatedBy string
}

type CreateIssueHandler struct {
	Store    issues.Store
	Enricher *services.IssueEnricher
	Events   issues.EventStore
}

func (h CreateIssueHandler) Handle(ctx context.Context, input CreateIssue) (issues.Issue, error) {
	enriched, err := h.Enricher.AnalyzeCreateInput(ctx, issues.CreateInput{
		Raw:       input.Raw,
		Tags:      input.Tags,
		CreatedBy: input.CreatedBy,
	})
	if err != nil {
		return issues.Issue{}, err
	}

	if _, err := issues.ValidateRaw(enriched.Raw, "raw"); err != nil {
		return issues.Issue{}, err
	}

	id, err := h.Store.NextIssueID(ctx)
	if err != nil {
		return issues.Issue{}, err
	}

	issue := issues.BuildNewIssue(id, enriched)
	if err := h.Store.SaveIssue(ctx, issue); err != nil {
		return issues.Issue{}, err
	}

	if h.Events != nil {
		_ = h.Events.RecordEvent(ctx, issues.Event{
			ID:        issues.IssuePostID(id, 1),
			Kind:      "report",
			IssueID:   id,
			CreatedBy: issue.CreatedBy,
			CreatedAt: issue.CreatedAt,
			Body:      issue.Raw,
		})
	}

	return issue, nil
}
