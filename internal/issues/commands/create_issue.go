package commands

import (
	"context"
	"fmt"
	"log/slog"

	issueenrichment "splat/internal/issueenrichment"
	"splat/internal/issues"
)

type CreateIssue struct {
	Raw       string
	Tags      []string
	CreatedBy string
}

type CreateIssueHandler struct {
	Logger   *slog.Logger
	Runner   *CommandRunner
	Enricher *issueenrichment.IssueEnricher
	Events   issues.EventPublisher
}

func (h CreateIssueHandler) Handle(ctx context.Context, input CreateIssue) (created issues.Issue, err error) {
	if _, err := issues.ValidateRaw(input.Raw, "raw"); err != nil {
		return issues.Issue{}, err
	}

	id := issues.NewIssueID()

	h.Logger.InfoContext(ctx, "creating issue",
		"issue_id", id,
		"created_by", input.CreatedBy,
		"tag_count", len(input.Tags),
	)

	issue := issues.BuildNewIssue(id, issues.CreateInput{
		Raw:       input.Raw,
		Tags:      input.Tags,
		CreatedBy: input.CreatedBy,
	})
	issue.EnrichmentStatus = issues.EnrichmentStatusPending
	issue.EnrichmentError = ""
	issue.EnrichmentTargetSequence = 1
	reportEvent := issue.ReportEvent()

	uow, finish, err := Begin(ctx, h.Runner)
	if err != nil {
		return issues.Issue{}, err
	}

	defer FinishAndThen(&err, finish, func() {
		h.Logger.InfoContext(ctx, "issue created", "issue_id", id)
		h.Events.PublishOne(ctx, reportEvent)
	})

	if err := uow.SaveIssue(ctx, issue); err != nil {
		return issues.Issue{}, err
	}
	jobStore, ok := uow.(issues.EnrichmentJobWriter)
	if !ok {
		return issues.Issue{}, fmt.Errorf("unit of work does not support issue enrichment jobs")
	}
	if err := jobStore.EnqueueIssueEnrichment(ctx, issue.ID, issue.EnrichmentTargetSequence); err != nil {
		return issues.Issue{}, err
	}

	_ = uow.RecordEvent(ctx, reportEvent)

	return issue, nil
}
