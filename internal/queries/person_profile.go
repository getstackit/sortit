package queries

import (
	"context"
	"strings"

	"splat/internal/domain"
	"splat/internal/issues"
)

// TagRelevance is an alias for domain.TagRelevance.
type TagRelevance = domain.TagRelevance

type PersonTagProfile struct {
	Person     string         `json:"person"`
	IssueCount int            `json:"issueCount"`
	TagProfile []TagRelevance `json:"tagProfile"`
}

type GetPersonProfileHandler struct {
	Store issues.Store
}

func (h GetPersonProfileHandler) Handle(ctx context.Context, person string, filter IssueStatusFilter) (PersonTagProfile, error) {
	person = strings.TrimSpace(person)
	if person == "" {
		return PersonTagProfile{}, nil
	}

	if store, ok := h.Store.(filteredIssueLister); ok {
		items, err := store.ListFiltered(ctx, issues.ListOptions{
			Status:     issueStatusFromFilter(filter),
			AssignedTo: person,
		})
		if err != nil {
			return PersonTagProfile{}, err
		}
		return buildPersonTagProfile(items, person, IssueStatusFilterAll), nil
	}

	if h.Store != nil {
		allIssues, err := h.Store.List(ctx)
		if err != nil {
			return PersonTagProfile{}, err
		}
		return buildPersonTagProfile(allIssues, person, filter), nil
	}

	return PersonTagProfile{}, nil
}

func buildPersonTagProfile(allIssues []issues.Issue, person string, filter IssueStatusFilter) PersonTagProfile {
	allIssues = FilterIssuesByStatus(allIssues, filter)

	var matched []issues.Issue
	for _, issue := range allIssues {
		if strings.EqualFold(issue.AssignedTo, person) {
			matched = append(matched, issue)
		}
	}

	return PersonTagProfile{
		Person:     person,
		IssueCount: len(matched),
		TagProfile: meanTagProfile(matched),
	}
}
