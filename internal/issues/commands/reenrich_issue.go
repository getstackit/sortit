package commands

import (
	"context"
	"fmt"
	"time"

	"sortit/internal/issues"
)

type ReEnrichIssue struct {
	ID string
}

type ReEnrichIssueHandler struct {
	Runner *CommandRunner
	Store  issues.Store
	Events issues.EventPublisher
}

func (h ReEnrichIssueHandler) Handle(ctx context.Context, input ReEnrichIssue) (reEnriched issues.Issue, err error) {
	current, err := h.Store.Get(ctx, input.ID)
	if err != nil {
		return issues.Issue{}, err
	}

	if err := issues.EnsureMutable(current); err != nil {
		return issues.Issue{}, err
	}

	uow, finish, err := Begin(ctx, h.Runner)
	if err != nil {
		return issues.Issue{}, err
	}

	var reEnrichEvent issues.Event
	defer func() {
		FinishAndPublish(&err, finish, ctx, h.Events, reEnrichEvent)
	}()

	current, err = uow.Get(ctx, input.ID)
	if err != nil {
		return issues.Issue{}, err
	}
	if err := issues.EnsureMutable(current); err != nil {
		return issues.Issue{}, err
	}

	jobStore, ok := uow.(issues.EnrichmentJobWriter)
	if !ok {
		return issues.Issue{}, fmt.Errorf("unit of work does not support issue enrichment jobs")
	}

	pendingStatus := issues.EnrichmentStatusPending
	emptyError := ""
	targetSequence := current.EnrichmentTargetSequence

	if err := uow.UpdateIssueFields(ctx, current.ID, issues.IssueFieldUpdate{
		EnrichmentStatus:         &pendingStatus,
		EnrichmentError:          &emptyError,
		EnrichmentTargetSequence: &targetSequence,
	}); err != nil {
		return issues.Issue{}, err
	}
	if err := jobStore.EnqueueIssueEnrichment(ctx, current.ID, targetSequence); err != nil {
		return issues.Issue{}, err
	}

	now := time.Now().UTC()
	reEnrichEvent = issues.Event{
		ID:        issues.NewEnrichmentEventID(),
		Kind:      "re-enrich",
		IssueID:   current.ID,
		CreatedBy: "system",
		CreatedAt: now,
	}
	_ = uow.RecordEvent(ctx, reEnrichEvent)

	return uow.Get(ctx, current.ID)
}
