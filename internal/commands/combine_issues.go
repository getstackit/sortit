package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"splat/internal/issues"
	"splat/internal/services"
)

type CombineIssues struct {
	SourceIDs []string
	CreatedBy string
	Note      string
}

type CombineIssuesHandler struct {
	Store    issues.Store
	Enricher *services.IssueEnricher
	Events   issues.EventStore
}

func (h CombineIssuesHandler) Handle(ctx context.Context, input CombineIssues) (issues.IssueOperationResult, error) {
	sourceIDs := issues.SanitizeIssueIDs(input.SourceIDs)
	if len(sourceIDs) < 2 {
		return issues.IssueOperationResult{}, fmt.Errorf("at least two source issues are required")
	}

	sourceIssues := make([]issues.Issue, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		issue, err := h.Store.Get(ctx, id)
		if err != nil {
			return issues.IssueOperationResult{}, err
		}
		sourceIssues = append(sourceIssues, issue)
	}

	enriched, err := h.Enricher.AnalyzeCombineInput(ctx, sourceIssues, input.CreatedBy, input.Note)
	if err != nil {
		return issues.IssueOperationResult{}, err
	}

	raw := strings.TrimSpace(enriched.Raw)
	if raw == "" {
		return issues.IssueOperationResult{}, fmt.Errorf("raw is required")
	}

	actor := issues.DefaultActor(enriched.CreatedBy)
	note := strings.TrimSpace(enriched.Note)
	createdAt := time.Now().UTC()

	combinedID := issues.NewIssueID()

	combinedIssue := issues.BuildNewIssue(combinedID, issues.CreateInput{
		Raw:       raw,
		Tags:      enriched.Tags,
		CreatedBy: enriched.CreatedBy,
		TagScores: enriched.TagScores,
		Embedding: enriched.Embedding,
	})
	combinedIssue.CreatedAt = createdAt

	if err := h.Store.SaveIssue(ctx, combinedIssue); err != nil {
		return issues.IssueOperationResult{}, err
	}

	opID := issues.NewOperationID()

	operation := issues.IssueOperation{
		ID:        opID,
		Kind:      issues.IssueOperationKindCombine,
		CreatedBy: actor,
		CreatedAt: createdAt,
		Note:      note,
		Participants: []issues.IssueOperationParticipant{{
			IssueID: combinedID,
			Role:    "result",
		}},
	}

	allIssues := map[string]issues.Issue{combinedID: combinedIssue}
	touched := []issues.IssueReference{issues.IssueReferenceFrom(combinedIssue)}

	for index, id := range sourceIDs {
		status := issues.StatusClosed
		if err := h.Store.UpdateIssueFields(ctx, id, issues.IssueFieldUpdate{
			Status:   &status,
			ClosedAt: &createdAt,
			ClosedBy: &actor,
		}); err != nil {
			return issues.IssueOperationResult{}, err
		}

		source := sourceIssues[index]
		source.Status = issues.StatusClosed
		source.ClosedAt = &createdAt
		source.ClosedBy = actor
		allIssues[id] = source
		touched = append(touched, issues.IssueReferenceFrom(source))

		if err := h.Store.SaveLink(ctx, issues.IssueLink{
			ID:            fmt.Sprintf("%s-link-%06d", opID, index+1),
			Type:          issues.IssueLinkTypeMergedInto,
			SourceIssueID: id,
			TargetIssueID: combinedID,
			CreatedBy:     actor,
			CreatedAt:     createdAt,
			Note:          note,
			OperationID:   opID,
		}); err != nil {
			return issues.IssueOperationResult{}, err
		}

		operation.Participants = append(operation.Participants, issues.IssueOperationParticipant{
			IssueID: id,
			Role:    fmt.Sprintf("source:%d", index+1),
		})
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
			Kind:         "combine",
			IssueID:      combinedID,
			CreatedBy:    actor,
			CreatedAt:    createdAt,
			Body:         note,
			Participants: participants,
		})
	}

	return issues.IssueOperationResult{
		Operation:     issues.HydrateOperation(operation, allIssues),
		CreatedIssues: []issues.Issue{combinedIssue},
		TouchedIssues: touched,
	}, nil
}
