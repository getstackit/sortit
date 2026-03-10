package queries

import (
	"context"

	"splat/internal/issues"
)

type ListIssuesHandler struct {
	Store issues.Store
}

type ListIssuesQuery struct {
	Status     IssueStatusFilter
	AssignedTo string
	Tags       []string
	Limit      int
	Offset     int
}

func (h ListIssuesHandler) Handle(ctx context.Context, input ListIssuesQuery) ([]issues.Issue, error) {
	if input.Status == "" {
		input.Status = IssueStatusFilterOpen
	}

	if store, ok := h.Store.(filteredIssueLister); ok {
		return store.ListFiltered(ctx, issues.ListOptions{
			Status:     issueStatusFromFilter(input.Status),
			AssignedTo: input.AssignedTo,
			Tags:       input.Tags,
			Limit:      input.Limit,
			Offset:     input.Offset,
		})
	}

	items, err := h.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	items = FilterIssuesByStatus(items, input.Status)
	items = filterIssuesByAssignee(items, input.AssignedTo)
	items = filterIssuesByTags(items, input.Tags)
	return paginateIssues(items, input.Limit, input.Offset), nil
}
