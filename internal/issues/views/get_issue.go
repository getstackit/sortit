package issueviews

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"sortit/internal/issueanalytics"
	"sortit/internal/issues"
)

type GetIssue struct {
	ID string
}

type GetIssueHandler struct {
	Store  issues.IssueDetailStore
	Logger *slog.Logger
}

func (h GetIssueHandler) Handle(ctx context.Context, input GetIssue) (_ issues.Issue, err error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return issues.Issue{}, issues.ErrNotFound
	}

	start := time.Now()
	defer func() {
		if h.Logger == nil {
			return
		}
		h.Logger.InfoContext(ctx, "service call complete",
			"service", "GetIssueHandler.Handle",
			"issue_id", id,
			"duration", time.Since(start).Round(time.Millisecond),
		)
	}()

	issue, err := h.Store.GetIssueDetailBase(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}

	discussion, err := h.Store.ListIssueDetailPosts(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}
	links, err := h.Store.ListIssueDetailLinks(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}
	operations, err := h.Store.ListIssueDetailOperations(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}
	references, err := h.loadIssueReferences(ctx, issue, links, operations)
	if err != nil {
		return issues.Issue{}, err
	}

	issue.Discussion = hydrateIssueDiscussion(issue, discussion)
	issue.Links = hydrateIssueLinks(issue.ID, links, references)
	issue.Operations = hydrateIssueOperations(operations, references)
	snapshots, err := h.Store.ListIssueSnapshots(ctx, id)
	if err != nil {
		return issues.Issue{}, err
	}
	issue.LifecycleMetrics = issueanalytics.DeriveIssueLifecycleMetricsAt(issue.Raw, issue.Discussion, issue.Links, snapshots, time.Now().UTC())
	return issue, nil
}

func (h GetIssueHandler) loadIssueReferences(
	ctx context.Context,
	issue issues.Issue,
	links []issues.IssueLink,
	operations []issues.IssueOperation,
) (map[string]issues.IssueReference, error) {
	ids := make([]string, 0, len(links)+(len(operations)*2))
	for _, link := range links {
		ids = append(ids, link.SourceIssueID, link.TargetIssueID)
	}
	for _, operation := range operations {
		for _, participant := range operation.Participants {
			ids = append(ids, participant.IssueID)
		}
	}

	references := map[string]issues.IssueReference{
		issue.ID: issues.IssueReferenceFrom(issue),
	}
	refItems, err := h.Store.ListIssueDetailReferences(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, ref := range refItems {
		references[ref.ID] = ref
	}
	return references, nil
}

func hydrateIssueDiscussion(issue issues.Issue, discussion []issues.IssuePost) []issues.IssuePost {
	if len(discussion) == 0 {
		return issues.InitialDiscussion(issue)
	}
	return append([]issues.IssuePost(nil), discussion...)
}

func hydrateIssueLinks(
	issueID string,
	links []issues.IssueLink,
	references map[string]issues.IssueReference,
) []issues.IssueLink {
	hydrated := make([]issues.IssueLink, 0, len(links))
	for _, link := range links {
		item := link
		if item.SourceIssueID == issueID {
			item.Direction = "outgoing"
			if ref, ok := references[item.TargetIssueID]; ok {
				refCopy := ref
				item.RelatedIssue = &refCopy
			}
		} else {
			item.Direction = "incoming"
			if ref, ok := references[item.SourceIssueID]; ok {
				refCopy := ref
				item.RelatedIssue = &refCopy
			}
		}
		hydrated = append(hydrated, item)
	}
	return hydrated
}

func hydrateIssueOperations(
	operations []issues.IssueOperation,
	references map[string]issues.IssueReference,
) []issues.IssueOperation {
	hydrated := make([]issues.IssueOperation, 0, len(operations))
	for _, operation := range operations {
		item := operation
		participants := make([]issues.IssueOperationParticipant, 0, len(operation.Participants))
		for _, participant := range operation.Participants {
			entry := participant
			if ref, ok := references[participant.IssueID]; ok {
				refCopy := ref
				entry.Issue = &refCopy
			}
			participants = append(participants, entry)
		}
		item.Participants = participants
		hydrated = append(hydrated, item)
	}
	return hydrated
}
