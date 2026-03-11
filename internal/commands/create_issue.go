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
	Events   issues.EventPublisher
}

func (h CreateIssueHandler) Handle(ctx context.Context, input CreateIssue) (created issues.Issue, err error) {
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
	reportEvent := issue.ReportEvent()

	uow, finish, err := Begin(ctx, h.Runner)
	if err != nil {
		return issues.Issue{}, err
	}

	defer FinishAndThen(&err, finish, func() {
		h.Events.PublishOne(ctx, reportEvent)
	})

	if err := uow.SaveIssue(ctx, issue); err != nil {
		return issues.Issue{}, err
	}

	_ = uow.RecordEvent(ctx, reportEvent)

	return issue, nil
}
