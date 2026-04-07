package issueviews

import (
	"context"
	"errors"
	"strings"

	"splat/internal/issues"
	issuemap "splat/internal/map"
)

type IssueStatusFilter string

const (
	IssueStatusFilterOpen   IssueStatusFilter = "open"
	IssueStatusFilterClosed IssueStatusFilter = "closed"
	IssueStatusFilterAll    IssueStatusFilter = "all"
)

type FilteredIssueLister interface {
	ListFiltered(context.Context, issues.ListOptions) ([]issues.Issue, error)
}

type IssueMetadataLister interface {
	ListIssueMetadata(context.Context) ([]issues.Issue, error)
}

func FilterIssuesByStatus(items []issues.Issue, filter IssueStatusFilter) []issues.Issue {
	if filter == "" {
		filter = IssueStatusFilterOpen
	}
	if filter == IssueStatusFilterAll {
		return items
	}

	filtered := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		status := item.Status
		if status == "" {
			status = issues.StatusOpen
		}

		if filter == IssueStatusFilterOpen && status == issues.StatusOpen {
			filtered = append(filtered, item)
		}
		if filter == IssueStatusFilterClosed && status == issues.StatusClosed {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func IssueStatusFromFilter(filter IssueStatusFilter) issues.IssueStatus {
	switch filter {
	case IssueStatusFilterClosed:
		return issues.StatusClosed
	case IssueStatusFilterAll:
		return ""
	default:
		return issues.StatusOpen
	}
}

func FilterIssuesByAssignee(items []issues.Issue, assignedTo string) []issues.Issue {
	assignedTo = strings.TrimSpace(assignedTo)
	if assignedTo == "" {
		return items
	}
	filtered := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(item.AssignedTo, assignedTo) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func FilterIssuesByTags(items []issues.Issue, tags []string) []issues.Issue {
	if len(tags) == 0 {
		return items
	}
	required := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag != "" {
			required[tag] = struct{}{}
		}
	}
	if len(required) == 0 {
		return items
	}
	filtered := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		if issueMatchesTags(item, required) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func SubsetProjectionByStatus(projection issuemap.MapProjection, filter IssueStatusFilter) issuemap.MapProjection {
	ids := make(map[string]struct{}, len(projection.MapIssues))
	for _, item := range projection.MapIssues {
		status := item.Status
		if status == "" {
			status = issues.StatusOpen
		}
		switch filter {
		case IssueStatusFilterAll:
			ids[item.ID] = struct{}{}
		case IssueStatusFilterClosed:
			if status == issues.StatusClosed {
				ids[item.ID] = struct{}{}
			}
		default:
			if status == issues.StatusOpen {
				ids[item.ID] = struct{}{}
			}
		}
	}
	return issuemap.SubsetMapProjection(projection, ids)
}

func OverlayProjectionIssueMetadata(
	projection issuemap.MapProjection,
	items []issues.Issue,
) issuemap.MapProjection {
	if len(projection.MapIssues) == 0 || len(items) == 0 {
		return projection
	}

	itemsByID := make(map[string]issues.Issue, len(items))
	for _, item := range items {
		itemsByID[item.ID] = item
	}

	mapIssues := append([]issuemap.MapIssue(nil), projection.MapIssues...)
	for i, mapIssue := range mapIssues {
		item, ok := itemsByID[mapIssue.ID]
		if !ok {
			continue
		}

		mapIssues[i].Raw = item.Raw
		mapIssues[i].Status = item.Status
		mapIssues[i].AssignedTo = item.AssignedTo
	}

	projection.MapIssues = mapIssues
	return projection
}

func PaginateIssues(items []issues.Issue, limit, offset int) []issues.Issue {
	if offset > 0 {
		if offset >= len(items) {
			return []issues.Issue{}
		}
		items = items[offset:]
	}
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}

func ListProjectionIssueMetadata(ctx context.Context, store issues.Store) ([]issues.Issue, error) {
	if metadataStore, ok := store.(IssueMetadataLister); ok {
		return metadataStore.ListIssueMetadata(ctx)
	}
	return store.List(ctx)
}

func issueMatchesTags(item issues.Issue, required map[string]struct{}) bool {
	for _, tag := range item.Tags {
		if _, ok := required[strings.ToLower(tag)]; ok {
			return true
		}
	}
	for _, ts := range item.TagScores {
		if ts.Relevance < 0.3 {
			continue
		}
		if _, ok := required[strings.ToLower(ts.Tag)]; ok {
			return true
		}
	}
	return false
}

func NotFound(err error) bool {
	return errors.Is(err, issues.ErrNotFound)
}
