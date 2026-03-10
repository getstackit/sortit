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
	Store    issues.Store
	Enricher *services.IssueEnricher
	Events   issues.EventStore
}

func (h SplitIssueHandler) Handle(ctx context.Context, input SplitIssue) (issues.IssueOperationResult, error) {
	sourceID := strings.TrimSpace(input.SourceID)
	if sourceID == "" {
		return issues.IssueOperationResult{}, issues.ErrNotFound
	}
	if len(input.Children) == 0 {
		return issues.IssueOperationResult{}, fmt.Errorf("at least one child issue is required")
	}

	source, err := h.Store.Get(ctx, sourceID)
	if err != nil {
		return issues.IssueOperationResult{}, err
	}

	actor := issues.DefaultActor(input.CreatedBy)
	note := strings.TrimSpace(input.Note)
	createdAt := time.Now().UTC()

	opID, err := h.Store.NextOperationID(ctx)
	if err != nil {
		return issues.IssueOperationResult{}, err
	}

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

	createdIssues := make([]issues.Issue, 0, len(input.Children))
	touched := []issues.IssueReference{issues.IssueReferenceFrom(source)}
	allIssues := map[string]issues.Issue{sourceID: source}

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

		childID, err := h.Store.NextIssueID(ctx)
		if err != nil {
			return issues.IssueOperationResult{}, err
		}

		childIssue := issues.BuildNewIssue(childID, enriched)
		childIssue.CreatedAt = createdAt

		if err := h.Store.SaveIssue(ctx, childIssue); err != nil {
			return issues.IssueOperationResult{}, err
		}

		createdIssues = append(createdIssues, childIssue)
		touched = append(touched, issues.IssueReferenceFrom(childIssue))
		allIssues[childID] = childIssue

		// Create parent/child links
		if err := h.Store.SaveLink(ctx, issues.IssueLink{
			ID:            fmt.Sprintf("%s-link-%06d", opID, index*2+1),
			Type:          issues.IssueLinkTypeParentOf,
			SourceIssueID: sourceID,
			TargetIssueID: childID,
			CreatedBy:     actor,
			CreatedAt:     createdAt,
			Note:          note,
			OperationID:   opID,
		}); err != nil {
			return issues.IssueOperationResult{}, err
		}
		if err := h.Store.SaveLink(ctx, issues.IssueLink{
			ID:            fmt.Sprintf("%s-link-%06d", opID, index*2+2),
			Type:          issues.IssueLinkTypeChildOf,
			SourceIssueID: childID,
			TargetIssueID: sourceID,
			CreatedBy:     actor,
			CreatedAt:     createdAt,
			Note:          note,
			OperationID:   opID,
		}); err != nil {
			return issues.IssueOperationResult{}, err
		}

		operation.Participants = append(operation.Participants, issues.IssueOperationParticipant{
			IssueID: childID,
			Role:    fmt.Sprintf("child:%d", index+1),
		})
	}

	if input.CloseSource {
		status := issues.StatusClosed
		if err := h.Store.UpdateIssueFields(ctx, sourceID, issues.IssueFieldUpdate{
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

	if err := h.Store.SaveOperation(ctx, operation); err != nil {
		return issues.IssueOperationResult{}, err
	}

	if h.Events != nil {
		participants := make([]issues.EventParticipant, 0, len(operation.Participants))
		for _, p := range operation.Participants {
			participants = append(participants, issues.EventParticipant{
				IssueID: p.IssueID,
				Role:    p.Role,
			})
		}
		_ = h.Events.RecordEvent(ctx, issues.Event{
			ID:           opID,
			Kind:         "split",
			IssueID:      sourceID,
			CreatedBy:    actor,
			CreatedAt:    createdAt,
			Body:         note,
			Participants: participants,
		})
	}

	return issues.IssueOperationResult{
		Operation:     issues.HydrateOperation(operation, allIssues),
		CreatedIssues: createdIssues,
		TouchedIssues: touched,
	}, nil
}
