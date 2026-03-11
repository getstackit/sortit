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
	Runner *CommandRunner
	Events issues.EventPublisher
}

func (h LinkIssuesHandler) Handle(ctx context.Context, input LinkIssues) (result issues.IssueOperationResult, err error) {
	sourceID := strings.TrimSpace(input.SourceID)
	targetID := strings.TrimSpace(input.TargetID)
	if sourceID == "" || targetID == "" {
		return issues.IssueOperationResult{}, issues.ErrNotFound
	}
	linkType := issues.NormalizeIssueLinkType(input.Type)
	if linkType == "" {
		return issues.IssueOperationResult{}, fmt.Errorf("link type is required")
	}

	uow, finish, err := Begin(ctx, h.Runner)
	if err != nil {
		return issues.IssueOperationResult{}, err
	}

	var linkEvent issues.Event
	defer func() {
		FinishAndPublish(&err, finish, ctx, h.Events, linkEvent)
	}()

	source, err := uow.Get(ctx, sourceID)
	if err != nil {
		return issues.IssueOperationResult{}, err
	}
	target, err := uow.Get(ctx, targetID)
	if err != nil {
		return issues.IssueOperationResult{}, err
	}

	actor := issues.DefaultActor(input.CreatedBy)
	note := strings.TrimSpace(input.Note)
	createdAt := time.Now().UTC()

	opID := issues.NewOperationID()

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

	if err := uow.SaveOperation(ctx, operation); err != nil {
		return issues.IssueOperationResult{}, err
	}

	if err := uow.SaveLink(ctx, issues.IssueLink{
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

	linkEvent = issues.Event{
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
	}
	_ = uow.RecordEvent(ctx, linkEvent)

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
