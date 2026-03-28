package queries

import (
	"context"
	"strings"

	"splat/internal/domain"
	"splat/internal/issuemath"
	"splat/internal/issues"
	"splat/internal/services"
)

// TagRelevance is an alias for domain.TagRelevance.
type TagRelevance = domain.TagRelevance

type PersonTagProfile struct {
	Person     string         `json:"person"`
	IssueCount int            `json:"issueCount"`
	TagProfile []TagRelevance `json:"tagProfile"`
}

type GetPersonProfileHandler struct {
	Store   issues.Store
	Catalog *services.CatalogService
}

type peopleAnalyticsLister interface {
	ListPeopleAnalytics(context.Context, issues.ListOptions) ([]issues.PeopleAnalyticsIssue, error)
}

func (h GetPersonProfileHandler) Handle(ctx context.Context, person string, filter IssueStatusFilter) (PersonTagProfile, error) {
	person = strings.TrimSpace(person)
	if person == "" {
		return PersonTagProfile{}, nil
	}

	tagSpecificity := loadTagSpecificityMap(ctx, h.Catalog)

	if store, ok := h.Store.(peopleAnalyticsLister); ok {
		items, err := store.ListPeopleAnalytics(ctx, issues.ListOptions{
			Status:     issueStatusFromFilter(filter),
			AssignedTo: person,
		})
		if err != nil {
			return PersonTagProfile{}, err
		}
		return buildPersonTagProfile(items, person, IssueStatusFilterAll, tagSpecificity), nil
	}

	if store, ok := h.Store.(filteredIssueLister); ok {
		items, err := store.ListFiltered(ctx, issues.ListOptions{
			Status:     issueStatusFromFilter(filter),
			AssignedTo: person,
		})
		if err != nil {
			return PersonTagProfile{}, err
		}
		return buildPersonTagProfile(peopleAnalyticsIssuesFromIssues(items), person, IssueStatusFilterAll, tagSpecificity), nil
	}

	if h.Store != nil {
		allIssues, err := h.Store.List(ctx)
		if err != nil {
			return PersonTagProfile{}, err
		}
		return buildPersonTagProfile(peopleAnalyticsIssuesFromIssues(allIssues), person, filter, tagSpecificity), nil
	}

	return PersonTagProfile{}, nil
}

func buildPersonTagProfile(allIssues []issues.PeopleAnalyticsIssue, person string, filter IssueStatusFilter, tagSpecificity map[string]*float64) PersonTagProfile {
	allIssues = filterPeopleAnalyticsByStatus(allIssues, filter)

	var matched []issues.PeopleAnalyticsIssue
	for _, issue := range allIssues {
		if strings.EqualFold(issue.AssignedTo, person) {
			matched = append(matched, issue)
		}
	}

	return PersonTagProfile{
		Person:     person,
		IssueCount: len(matched),
		TagProfile: issuemath.MeanTagProfile(matched, tagSpecificity),
	}
}
