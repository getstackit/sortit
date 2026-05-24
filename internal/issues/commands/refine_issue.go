package commands

import (
	"context"
	"fmt"

	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
)

type RefineIssue struct {
	ID        string
	Raw       string
	CreatedBy string
}

type RefineIssueHandler struct {
	Runner   *CommandRunner
	Store    issues.Store
	Enricher *issueenrichment.IssueEnricher
	Events   issues.EventPublisher
}

func (h RefineIssueHandler) Handle(ctx context.Context, input RefineIssue) (refined issues.Issue, err error) {
	postRaw, err := issues.ValidateRaw(input.Raw, "raw")
	if err != nil {
		return issues.Issue{}, err
	}

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

	post := issues.NewDiscussionPost(current.ID, current.Discussion, postRaw, issues.DefaultActor(input.CreatedBy), "refinement")
	if err := uow.SaveIssuePost(ctx, post); err != nil {
		return issues.Issue{}, err
	}
	jobStore, ok := uow.(issues.EnrichmentJobWriter)
	if !ok {
		return issues.Issue{}, fmt.Errorf("unit of work does not support issue enrichment jobs")
	}
	pendingStatus := issues.EnrichmentStatusPending
	emptyError := ""
	targetSequence := post.Sequence

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
