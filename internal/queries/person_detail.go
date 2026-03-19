package queries

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"splat/internal/issues"
	"splat/internal/scoring"
	"splat/internal/services"
	"splat/internal/vectors"
)

type PersonIssueRecommendation struct {
	Issue         issues.Issue `json:"issue"`
	Source        string       `json:"source"`
	Reason        string       `json:"reason"`
	CombinedScore float64      `json:"combinedScore"`
	SemanticScore float64      `json:"semanticScore"`
	FactorScore   float64      `json:"factorScore"`
	SharedTags    []string     `json:"sharedTags"`
}

type PersonDetail struct {
	Person            string                      `json:"person"`
	IssueCount        int                         `json:"issueCount"`
	OpenIssueCount    int                         `json:"openIssueCount"`
	ClosedIssueCount  int                         `json:"closedIssueCount"`
	TagProfile        []TagRelevance              `json:"tagProfile"`
	AssignedIssues    []issues.Issue              `json:"assignedIssues"`
	NextIssue         *PersonIssueRecommendation  `json:"nextIssue,omitempty"`
	RecommendedIssues []PersonIssueRecommendation `json:"recommendedIssues"`
}

type GetPersonDetailHandler struct {
	Store   issues.Store
	Catalog *services.CatalogService
}

func (h GetPersonDetailHandler) Handle(ctx context.Context, person string) (PersonDetail, error) {
	person = strings.TrimSpace(person)
	if person == "" {
		return PersonDetail{}, nil
	}

	assignedIssues, err := h.listAssignedIssues(ctx, person)
	if err != nil {
		return PersonDetail{}, err
	}

	tagSpecificity := loadTagSpecificityMap(ctx, h.Catalog)
	profile := buildPersonTagProfile(peopleAnalyticsIssuesFromIssues(assignedIssues), person, IssueStatusFilterAll, tagSpecificity)
	openAssigned, closedAssigned := splitIssuesByStatus(assignedIssues)
	sortAssignedIssues(openAssigned)
	sortAssignedIssues(closedAssigned)

	detail := PersonDetail{
		Person:            profile.Person,
		IssueCount:        profile.IssueCount,
		OpenIssueCount:    len(openAssigned),
		ClosedIssueCount:  len(closedAssigned),
		TagProfile:        profile.TagProfile,
		AssignedIssues:    append(append([]issues.Issue{}, openAssigned...), closedAssigned...),
		RecommendedIssues: []PersonIssueRecommendation{},
	}

	if len(openAssigned) > 0 {
		detail.NextIssue = &PersonIssueRecommendation{
			Issue:         openAssigned[0],
			Source:        "assigned",
			Reason:        "Oldest open issue already assigned to this person",
			CombinedScore: 1,
			SemanticScore: 1,
			FactorScore:   1,
			SharedTags:    topSharedTags(profile.TagProfile, issueTagProfile(openAssigned[0]), 3),
		}
	}

	recommendations, err := h.recommendOpenIssues(ctx, person, profile.TagProfile, meanEmbedding(peopleAnalyticsIssuesFromIssues(assignedIssues)))
	if err != nil {
		return PersonDetail{}, err
	}
	detail.RecommendedIssues = recommendations
	if detail.NextIssue == nil && len(recommendations) > 0 {
		recommendation := recommendations[0]
		detail.NextIssue = &recommendation
	}

	return detail, nil
}

func (h GetPersonDetailHandler) listAssignedIssues(ctx context.Context, person string) ([]issues.Issue, error) {
	if store, ok := h.Store.(filteredIssueLister); ok {
		return store.ListFiltered(ctx, issues.ListOptions{
			AssignedTo: person,
		})
	}

	items, err := h.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	return filterIssuesByAssignee(items, person), nil
}

func (h GetPersonDetailHandler) recommendOpenIssues(
	ctx context.Context,
	person string,
	profile []TagRelevance,
	personEmbedding []float64,
) ([]PersonIssueRecommendation, error) {
	var openIssues []issues.Issue
	var err error
	if store, ok := h.Store.(filteredIssueLister); ok {
		openIssues, err = store.ListFiltered(ctx, issues.ListOptions{Status: issues.StatusOpen})
	} else {
		openIssues, err = h.Store.List(ctx)
		if err == nil {
			openIssues = FilterIssuesByStatus(openIssues, IssueStatusFilterOpen)
		}
	}
	if err != nil {
		return nil, err
	}

	recommendations := make([]PersonIssueRecommendation, 0, len(openIssues))
	detailReader, _ := h.Store.(issues.IssueDetailReader)
	now := time.Now().UTC()
	for _, issue := range openIssues {
		if strings.EqualFold(issue.AssignedTo, person) || strings.TrimSpace(issue.AssignedTo) != "" {
			continue
		}
		issue = hydrateIssueWithVelocity(ctx, detailReader, issue, now)

		issueTags := issueTagProfile(issue)
		factorScore := tagProfileSimilarity(profile, issueTags)
		semanticScore := vectors.CosineSimilarity(personEmbedding, issue.Embedding)
		combinedScore := scoring.PersonRecommendFactor*factorScore + scoring.PersonRecommendSemantic*semanticScore
		combinedScore *= issues.IssueFreshnessWeight(issue, now)
		combinedScore *= scoring.PersonMaturityBase + scoring.PersonMaturityWeight*issuesMaturity(issue)
		combinedScore *= 1 - scoring.PersonVelocityPenalty*issueVelocity(issue)
		combinedScore += issues.IssueAuthority(issue) * scoring.AuthorityConsumerWt
		sharedTags := topSharedTags(profile, issueTags, scoring.SharedTagsLimit)
		reason := recommendationReason(sharedTags, factorScore, semanticScore)
		recommendations = append(recommendations, PersonIssueRecommendation{
			Issue:         issue,
			Source:        "recommended",
			Reason:        reason,
			CombinedScore: roundTo2(combinedScore),
			SemanticScore: roundTo2(semanticScore),
			FactorScore:   roundTo2(factorScore),
			SharedTags:    sharedTags,
		})
	}

	recommendations = filterLowSignalRecommendations(recommendations)
	sortRecommendations(recommendations)
	if len(recommendations) > scoring.PersonMaxRecommend {
		recommendations = recommendations[:scoring.PersonMaxRecommend]
	}
	return recommendations, nil
}

func issuesMaturity(issue issues.Issue) float64 {
	return issuesLifecycleMaturity(issue.LifecycleMetrics)
}

func issueVelocity(issue issues.Issue) float64 {
	return issuesLifecycleVelocity(issue.LifecycleMetrics)
}

func issuesLifecycleMaturity(metrics *issues.IssueLifecycleMetrics) float64 {
	if metrics != nil && metrics.Maturity != nil {
		return roundTo2(*metrics.Maturity)
	}
	return scoring.DefaultMaturity
}

func issuesLifecycleVelocity(metrics *issues.IssueLifecycleMetrics) float64 {
	if metrics != nil && metrics.Velocity != nil {
		return roundTo2(*metrics.Velocity)
	}
	return 0
}

func issueTagProfile(item issues.Issue) []TagRelevance {
	if len(item.TagScores) > 0 {
		return append([]TagRelevance(nil), item.TagScores...)
	}

	if len(item.Tags) == 0 {
		return []TagRelevance{}
	}

	profile := make([]TagRelevance, 0, len(item.Tags))
	seen := make(map[string]struct{}, len(item.Tags))
	for _, tag := range item.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		profile = append(profile, TagRelevance{Tag: tag, Relevance: 1})
	}
	return profile
}

func splitIssuesByStatus(items []issues.Issue) ([]issues.Issue, []issues.Issue) {
	openItems := make([]issues.Issue, 0, len(items))
	closedItems := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		if item.Status == issues.StatusClosed {
			closedItems = append(closedItems, item)
			continue
		}
		openItems = append(openItems, item)
	}
	return openItems, closedItems
}

func sortAssignedIssues(items []issues.Issue) {
	slices.SortStableFunc(items, func(a, b issues.Issue) int {
		aTime := a.CreatedAt
		bTime := b.CreatedAt
		if diff := aTime.Compare(bTime); diff != 0 {
			return diff
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

func filterLowSignalRecommendations(items []PersonIssueRecommendation) []PersonIssueRecommendation {
	if len(items) == 0 {
		return items
	}

	filtered := make([]PersonIssueRecommendation, 0, len(items))
	for _, item := range items {
		if item.FactorScore < scoring.PersonLowSignal && item.SemanticScore < scoring.PersonLowSignal {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func sortRecommendations(items []PersonIssueRecommendation) {
	slices.SortStableFunc(items, func(a, b PersonIssueRecommendation) int {
		if diff := cmp.Compare(b.CombinedScore, a.CombinedScore); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.FactorScore, a.FactorScore); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.SemanticScore, a.SemanticScore); diff != 0 {
			return diff
		}
		aTime := a.Issue.CreatedAt
		bTime := b.Issue.CreatedAt
		if diff := aTime.Compare(bTime); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Issue.ID, b.Issue.ID)
	})
}

func topSharedTags(profile, issueTags []TagRelevance, limit int) []string {
	shared := sharedTags(profile, issueTags)
	if len(shared) > limit {
		shared = shared[:limit]
	}
	return shared
}

func recommendationReason(sharedTags []string, factorScore, semanticScore float64) string {
	switch {
	case len(sharedTags) > 0:
		return "Matches this person's historical focus in " + strings.Join(sharedTags, ", ")
	case factorScore >= scoring.PersonReasonFactor:
		return "Strong factor-profile match to prior assigned work"
	case semanticScore >= scoring.PersonReasonSemantic:
		return "Semantically close to the language used in this person's prior work"
	default:
		return "Best available open issue based on blended profile similarity"
	}
}
