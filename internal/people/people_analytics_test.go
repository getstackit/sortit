package people

import (
	"context"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
	issueviews "sortit/internal/issues/views"
)

type peopleAnalyticsOnlyStore struct {
	items []issues.PeopleAnalyticsIssue
}

func (s peopleAnalyticsOnlyStore) List(_ context.Context) ([]issues.Issue, error) {
	panic("List should not be called when people analytics store is available")
}

func (s peopleAnalyticsOnlyStore) Get(_ context.Context, _ string) (issues.Issue, error) {
	panic("Get should not be called in this test")
}

func (s peopleAnalyticsOnlyStore) SaveIssue(context.Context, issues.Issue) error { return nil }
func (s peopleAnalyticsOnlyStore) SaveIssuePost(context.Context, issues.IssuePost) error {
	return nil
}
func (s peopleAnalyticsOnlyStore) UpdateIssueFields(context.Context, string, issues.IssueFieldUpdate) error {
	return nil
}
func (s peopleAnalyticsOnlyStore) SaveOperation(context.Context, issues.IssueOperation) error {
	return nil
}
func (s peopleAnalyticsOnlyStore) SaveLink(context.Context, issues.IssueLink) error { return nil }

func (s peopleAnalyticsOnlyStore) ListPeopleAnalytics(
	_ context.Context,
	_ issues.ListOptions,
) ([]issues.PeopleAnalyticsIssue, error) {
	return append([]issues.PeopleAnalyticsIssue(nil), s.items...), nil
}

func TestGetPersonProfileUsesPeopleAnalyticsStore(t *testing.T) {
	handler := GetPersonProfileHandler{
		Store: peopleAnalyticsOnlyStore{
			items: []issues.PeopleAnalyticsIssue{
				{
					Status:     issues.StatusOpen,
					AssignedTo: "Casey",
					TagScores:  []domain.TagRelevance{{Tag: "search", Relevance: 0.9}},
				},
				{
					Status:     issues.StatusOpen,
					AssignedTo: "Casey",
					TagScores:  []domain.TagRelevance{{Tag: "backend", Relevance: 0.5}},
				},
			},
		},
	}

	profile, err := handler.Handle(context.Background(), "Casey", issueviews.IssueStatusFilterOpen)
	if err != nil {
		t.Fatalf("handle profile: %v", err)
	}
	if profile.IssueCount != 2 {
		t.Fatalf("expected 2 issues, got %d", profile.IssueCount)
	}
	if len(profile.TagProfile) != 2 {
		t.Fatalf("expected 2 tags in profile, got %#v", profile.TagProfile)
	}
}

func TestWorkCorrelationsUsesPeopleAnalyticsStore(t *testing.T) {
	handler := WorkCorrelationsHandler{
		Store: peopleAnalyticsOnlyStore{
			items: []issues.PeopleAnalyticsIssue{
				{
					Status:     issues.StatusOpen,
					AssignedTo: "Casey",
					TagScores:  []domain.TagRelevance{{Tag: "search", Relevance: 1}},
					Embedding:  []float64{1, 0},
				},
				{
					Status:     issues.StatusOpen,
					AssignedTo: "Jordan",
					TagScores:  []domain.TagRelevance{{Tag: "search", Relevance: 0.8}},
					Embedding:  []float64{0.9, 0.1},
				},
			},
		},
	}

	result, err := handler.Handle(context.Background(), issueviews.IssueStatusFilterOpen)
	if err != nil {
		t.Fatalf("handle correlations: %v", err)
	}
	if len(result.Correlations) != 1 {
		t.Fatalf("expected 1 correlation, got %#v", result.Correlations)
	}
	if result.Correlations[0].PersonA == "" || result.Correlations[0].PersonB == "" {
		t.Fatalf("expected populated people, got %#v", result.Correlations[0])
	}
}
