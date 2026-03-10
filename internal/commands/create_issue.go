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
	Runner   *CommandRunner
	Enricher *services.IssueEnricher
}

func (h CreateIssueHandler) Handle(ctx context.Context, input CreateIssue) (issues.Issue, error) {
	if _, err := issues.ValidateRaw(input.Raw, "raw"); err != nil {
		return issues.Issue{}, err
	}

	id := issues.NewIssueID()

	enriched, err := h.Enricher.AnalyzeCreateInput(ctx, issues.CreateInput{
		Raw:       input.Raw,
		Tags:      input.Tags,
		CreatedBy: input.CreatedBy,
	})
	if err != nil {
		return issues.Issue{}, err
	}

	issue := issues.BuildNewIssue(id, enriched)

	return Run(ctx, h.Runner, func(ctx context.Context, uow issues.UnitOfWork) (issues.Issue, error) {
		if err := uow.SaveIssue(ctx, issue); err != nil {
			return issues.Issue{}, err
		}

		_ = uow.RecordEvent(ctx, issue.ReportEvent())

		return issue, nil
	})
}
