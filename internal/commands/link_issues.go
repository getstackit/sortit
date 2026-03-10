package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"splat/internal/issues"
)

type LinkIssues struct {
	SourceID  string
	TargetID  string
	Type      issues.IssueLinkType
	CreatedBy string
	Note      string
}

type LinkIssuesHandler struct {
	Store  issues.Store
	Events issues.EventStore
}

func (h LinkIssuesHandler) Handle(ctx context.Context, input LinkIssues) (issues.IssueOperationResult, error) {
	sourceID := strings.TrimSpace(input.SourceID)
	targetID := strings.TrimSpace(input.TargetID)
	if sourceID == "" || targetID == "" {
		return issues.IssueOperationResult{}, issues.ErrNotFound
	}
	linkType := issues.NormalizeIssueLinkType(input.Type)
	if linkType == "" {
		return issues.IssueOperationResult{}, fmt.Errorf("link type is required")
	}

	source, err := h.Store.Get(ctx, sourceID)
	if err != nil {
		return issues.IssueOperationResult{}, err
	}
	target, err := h.Store.Get(ctx, targetID)
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
		Kind:      issues.IssueOperationKindLink,
		CreatedBy: actor,
		CreatedAt: createdAt,
		Note:      note,
		Participants: []issues.IssueOperationParticipant{
			{IssueID: sourceID, Role: "source"},
			{IssueID: targetID, Role: "target"},
		},
	}

	if err := h.Store.SaveOperation(ctx, operation); err != nil {
		return issues.IssueOperationResult{}, err
	}

	if err := h.Store.SaveLink(ctx, issues.IssueLink{
		ID:            fmt.Sprintf("%s-link-000001", opID),
		Type:          linkType,
		SourceIssueID: sourceID,
		TargetIssueID: targetID,
		CreatedBy:     actor,
		CreatedAt:     createdAt,
		Note:          note,
		OperationID:   opID,
	}); err != nil {
		return issues.IssueOperationResult{}, err
	}

	if h.Events != nil {
		_ = h.Events.RecordEvent(ctx, issues.Event{
			ID:        opID,
			Kind:      "link",
			IssueID:   sourceID,
			CreatedBy: actor,
			CreatedAt: createdAt,
			Body:      note,
			Participants: []issues.EventParticipant{
				{IssueID: sourceID, Role: "source"},
				{IssueID: targetID, Role: "target"},
			},
		})
	}

	// Hydrate operation with current issue references
	allIssues := map[string]issues.Issue{
		sourceID: source,
		targetID: target,
	}
	hydrated := issues.HydrateOperation(operation, allIssues)

	return issues.IssueOperationResult{
		Operation: hydrated,
		TouchedIssues: []issues.IssueReference{
			issues.IssueReferenceFrom(source),
			issues.IssueReferenceFrom(target),
		},
	}, nil
}
