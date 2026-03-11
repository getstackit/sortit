package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"splat/internal/issues"
	"splat/internal/services"
)

type SplitIssueChild struct {
	Raw  string
	Tags []string
}

type SplitIssue struct {
	SourceID    string
	Children    []SplitIssueChild
	CreatedBy   string
	Note        string
	CloseSource bool
}

type SplitIssueHandler struct {
	Runner   *CommandRunner
	Enricher *services.IssueEnricher
	Events   issues.EventPublisher
}

func (h SplitIssueHandler) Handle(ctx context.Context, input SplitIssue) (result issues.IssueOperationResult, err error) {
	sourceID := strings.TrimSpace(input.SourceID)
	if sourceID == "" {
		return issues.IssueOperationResult{}, issues.ErrNotFound
	}
	if len(input.Children) == 0 {
		return issues.IssueOperationResult{}, fmt.Errorf("at least one child issue is required")
	}

	actor := issues.DefaultActor(input.CreatedBy)
	note := strings.TrimSpace(input.Note)
	createdAt := time.Now().UTC()

	opID := issues.NewOperationID()

	operation := issues.IssueOperation{
		ID:        opID,
		Kind:      issues.IssueOperationKindSplit,
		CreatedBy: actor,
		CreatedAt: createdAt,
		Note:      note,
		Participants: []issues.IssueOperationParticipant{{
			IssueID: sourceID,
			Role:    "source",
		}},
	}

	type childIssuePlan struct {
		issues.Issue
		index int
	}
	childPlans := make([]childIssuePlan, 0, len(input.Children))

	for index, child := range input.Children {
		enriched, err := h.Enricher.AnalyzeCreateInput(ctx, issues.CreateInput{
			Raw:       child.Raw,
			Tags:      child.Tags,
			CreatedBy: input.CreatedBy,
		})
		if err != nil {
			return issues.IssueOperationResult{}, err
		}

		raw := strings.TrimSpace(enriched.Raw)
		if raw == "" {
			return issues.IssueOperationResult{}, fmt.Errorf("child raw is required")
		}

		childID := issues.NewIssueID()

		childIssue := issues.BuildNewIssue(childID, enriched)
		childIssue.CreatedAt = createdAt

		operation.Participants = append(operation.Participants, issues.IssueOperationParticipant{
			IssueID: childID,
			Role:    fmt.Sprintf("child:%d", index+1),
		})
		childPlans = append(childPlans, childIssuePlan{
			Issue: childIssue,
			index: index,
		})
	}

	uow, finish, err := Begin(ctx, h.Runner)
	if err != nil {
		return issues.IssueOperationResult{}, err
	}

	var splitEvent issues.Event
	defer func() {
		FinishAndPublish(&err, finish, ctx, h.Events, splitEvent)
	}()

	source, err := uow.Get(ctx, sourceID)
	if err != nil {
		return issues.IssueOperationResult{}, err
	}

	createdIssues := make([]issues.Issue, 0, len(childPlans))
	touched := []issues.IssueReference{issues.IssueReferenceFrom(source)}
	allIssues := map[string]issues.Issue{sourceID: source}

	for _, childPlan := range childPlans {
		childIssue := childPlan.Issue

		if err := uow.SaveIssue(ctx, childIssue); err != nil {
			return issues.IssueOperationResult{}, err
		}

		createdIssues = append(createdIssues, childIssue)
		touched = append(touched, issues.IssueReferenceFrom(childIssue))
		allIssues[childIssue.ID] = childIssue

		// Create parent/child links.
		if err := uow.SaveLink(ctx, issues.IssueLink{
			ID:            fmt.Sprintf("%s-link-%06d", opID, childPlan.index*2+1),
			Type:          issues.IssueLinkTypeParentOf,
			SourceIssueID: sourceID,
			TargetIssueID: childIssue.ID,
			CreatedBy:     actor,
			CreatedAt:     createdAt,
			Note:          note,
			OperationID:   opID,
		}); err != nil {
			return issues.IssueOperationResult{}, err
		}
		if err := uow.SaveLink(ctx, issues.IssueLink{
			ID:            fmt.Sprintf("%s-link-%06d", opID, childPlan.index*2+2),
			Type:          issues.IssueLinkTypeChildOf,
			SourceIssueID: childIssue.ID,
			TargetIssueID: sourceID,
			CreatedBy:     actor,
			CreatedAt:     createdAt,
			Note:          note,
			OperationID:   opID,
		}); err != nil {
			return issues.IssueOperationResult{}, err
		}
	}

	if input.CloseSource {
		status := issues.StatusClosed
		if err := uow.UpdateIssueFields(ctx, sourceID, issues.IssueFieldUpdate{
			Status:   &status,
			ClosedAt: &createdAt,
			ClosedBy: &actor,
		}); err != nil {
			return issues.IssueOperationResult{}, err
		}
		source.Status = issues.StatusClosed
		source.ClosedAt = &createdAt
		source.ClosedBy = actor
		touched[0] = issues.IssueReferenceFrom(source)
		allIssues[sourceID] = source
	}

	if err := uow.SaveOperation(ctx, operation); err != nil {
		return issues.IssueOperationResult{}, err
	}

	participants := make([]issues.EventParticipant, 0, len(operation.Participants))
	for _, p := range operation.Participants {
		participants = append(participants, issues.EventParticipant{
			IssueID: p.IssueID,
			Role:    p.Role,
		})
	}
	splitEvent = issues.Event{
		ID:           opID,
		Kind:         "split",
		IssueID:      sourceID,
		CreatedBy:    actor,
		CreatedAt:    createdAt,
		Body:         note,
		Participants: participants,
	}
	_ = uow.RecordEvent(ctx, splitEvent)

	return issues.IssueOperationResult{
		Operation:     issues.HydrateOperation(operation, allIssues),
		CreatedIssues: createdIssues,
		TouchedIssues: touched,
	}, nil
}
