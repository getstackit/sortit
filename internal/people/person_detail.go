package people

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strings"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issueanalytics"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	issueviews "sortit/internal/issues/views"
	"sortit/internal/ridgedecomp"
	"sortit/internal/ridgelambda"
	"sortit/internal/scoring"
	"sortit/internal/tags"
	"sortit/internal/vectors"
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
	TagProfile        []domain.TagRelevance       `json:"tagProfile"`
	AssignedIssues    []issues.Issue              `json:"assignedIssues"`
	NextIssue         *PersonIssueRecommendation  `json:"nextIssue,omitempty"`
	RecommendedIssues []PersonIssueRecommendation `json:"recommendedIssues"`
}

type GetPersonDetailHandler struct {
	Store   issues.Store
	Catalog *tags.CatalogService
	// RidgeDecomp provides the revision-cached full-corpus ridge decomposition,
	// preferred over RidgeLambda: recommendations score candidates from the
	// cached per-issue vectors and decompose the person profile into the same
	// cached tag space, instead of re-solving over the open-issue set.
	RidgeDecomp *ridgedecomp.Cache
	// RidgeLambda selects the anchored-ridge similarity model for
	// recommendations when wired and the corpus is large enough; otherwise
	// recommendations use the rank-1 factor model. Mirrors the search path.
	// Production wires only RidgeDecomp (uncentered, WP-304): this in-place
	// solve runs in the centered open-issue space, so wire it only with a
	// centered-regime cache.
	RidgeLambda *ridgelambda.Cache
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
	profile := buildPersonTagProfile(peopleAnalyticsIssuesFromIssues(assignedIssues), person, issueviews.IssueStatusFilterAll, tagSpecificity)
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

	var storeTags []issues.Tag
	if h.Catalog != nil {
		storeTags, _ = h.Catalog.StoredTags(ctx)
	}
	recommendations, err := h.recommendOpenIssues(ctx, person, profile.TagProfile, issuemath.MeanEmbedding(peopleAnalyticsIssuesFromIssues(assignedIssues)), storeTags)
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
	if store, ok := h.Store.(issueviews.FilteredIssueLister); ok {
		return store.ListFiltered(ctx, issues.ListOptions{
			AssignedTo: person,
		})
	}

	items, err := h.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	return issueviews.FilterIssuesByAssignee(items, person), nil
}

func (h GetPersonDetailHandler) recommendOpenIssues(
	ctx context.Context,
	person string,
	profile []domain.TagRelevance,
	personEmbedding []float64,
	storeTags []issues.Tag,
) ([]PersonIssueRecommendation, error) {
	var openIssues []issues.Issue
	var err error
	if store, ok := h.Store.(issueviews.FilteredIssueLister); ok {
		openIssues, err = store.ListFiltered(ctx, issues.ListOptions{Status: issues.StatusOpen})
	} else {
		openIssues, err = h.Store.List(ctx)
		if err == nil {
			openIssues = issueviews.FilterIssuesByStatus(openIssues, issueviews.IssueStatusFilterOpen)
		}
	}
	if err != nil {
		return nil, err
	}

	// Build tag data for decomposition.
	tagNames, tagEmbeddings := personDetailTagData(openIssues, storeTags)

	// Build issue embeddings map and compute factor decomposition.
	issueEmbeddings := make(map[string][]float64, len(openIssues))
	for _, issue := range openIssues {
		if len(issue.Embedding) > 0 {
			issueEmbeddings[issue.ID] = issue.Embedding
		}
	}
	// Center against the corpus means. The open set is the corpus being
	// scored (not a similarity-retrieved subset), so self-derived means are
	// unbiased here. The person embedding is centered with the same issue
	// mean, never with its own statistics.
	rawPersonEmbedding := personEmbedding
	issueEmbeddings, tagEmbeddings, corpusMeans := issuemath.CenterEmbeddings(issueEmbeddings, tagEmbeddings)
	personEmbedding = issuemath.CenterVector(personEmbedding, corpusMeans.Issue)

	// Prefer the anchored-ridge model (Phase 3c) when it is available: first the
	// revision-cached full-corpus decomposition, then an in-place solve at the
	// GCV penalty, otherwise the rank-1 factor decomposition. Only the chosen
	// model is computed.
	var (
		ridgeDecomp      issuemath.RidgeDecomposition
		personRidge      issuemath.RidgeVectors
		decomp           issuemath.FactorDecomposition
		personDecomposed issuemath.DecomposedEmbedding
		useRidge         bool
	)
	if h.RidgeDecomp != nil {
		cached, ok, derr := h.RidgeDecomp.Current(ctx)
		if derr != nil {
			return nil, derr
		}
		if ok {
			// Cached path: candidate vectors and tag space come from the full
			// corpus. Center the person profile with the same corpus means so it
			// decomposes into that space, then blend against cached vectors.
			ridgeDecomp = cached.Decomposition
			personForCache := issuemath.CenterVector(rawPersonEmbedding, cached.Means.Issue)
			personRidge = cached.DecomposeQuery(personForCache, profile)
			useRidge = true
		}
	}
	if !useRidge && h.RidgeLambda != nil {
		lambda, ok, lerr := h.RidgeLambda.Current(ctx)
		if lerr != nil {
			return nil, lerr
		}
		if ok {
			ridgeDecomp = issuemath.ComputeRidgeDecomposition(openIssues, tagNames, issueEmbeddings, tagEmbeddings,
				scoring.RidgeAnchorLambdaScored, lambda)
			if ridgeDecomp.Decomposed() {
				personRidge = issuemath.DecomposeRidgeEmbedding(personEmbedding, profile, tagNames, tagEmbeddings,
					scoring.RidgeAnchorLambdaScored, lambda)
				useRidge = true
			}
		}
	}
	if !useRidge {
		decomp = issuemath.ComputeFactorDecomposition(openIssues, tagNames, issueEmbeddings, tagEmbeddings)
		tagCov := issuemath.BuildTagCovariance(tagNames, tagEmbeddings)
		personDecomposed = issuemath.DecomposeEmbedding(personEmbedding, profile, tagNames, tagEmbeddings, tagCov)
	}

	// blendPerson scores one candidate against the person, using whichever
	// similarity model is active and falling back to plain tag-profile +
	// embedding similarity when the candidate wasn't decomposed.
	blendPerson := func(issueID string, issueTags []domain.TagRelevance) (factor, semantic, combined float64) {
		if useRidge {
			if rv, ok := ridgeDecomp.VectorsFor(issueID); ok {
				return issuemath.RidgeBlend(ridgeDecomp, personRidge, rv, issuemath.RidgeTagSpace)
			}
		} else if issueDecomposed, ok := decomp.DecomposedFor(issueID); ok {
			return issuemath.BlendFromDecomposition(decomp, personDecomposed, issueDecomposed)
		}
		factor = issuemath.TagProfileSimilarity(profile, issueTags)
		semantic = vectors.CosineSimilarity(personEmbedding, issueEmbeddings[issueID])
		combined = scoring.PersonRecommendFactor*factor + scoring.PersonRecommendSemantic*semantic
		return factor, semantic, combined
	}

	recommendations := make([]PersonIssueRecommendation, 0, len(openIssues))
	detailReader, _ := h.Store.(issues.IssueDetailReader)
	now := time.Now().UTC()
	for _, issue := range openIssues {
		if strings.EqualFold(issue.AssignedTo, person) || strings.TrimSpace(issue.AssignedTo) != "" {
			continue
		}
		issue = issueviews.HydrateIssueWithVelocity(ctx, detailReader, issue, now)

		issueTags := issueTagProfile(issue)

		factorScore, semanticScore, combinedScore := blendPerson(issue.ID, issueTags)

		// Fit evidence clamps at zero, then candidate quality modulates
		// multiplicatively — the same composition rule as search. A
		// previously additive authority term could resurrect a candidate
		// with no fit evidence at all.
		combinedScore = math.Max(0, combinedScore)
		combinedScore *= issueanalytics.IssueFreshnessWeight(issue, now)
		combinedScore *= scoring.PersonMaturityBase + scoring.PersonMaturityWeight*issuesMaturity(issue)
		combinedScore *= 1 - scoring.PersonVelocityPenalty*issueVelocity(issue)
		combinedScore *= 1 + scoring.AuthorityConsumerWt*issueanalytics.IssueAuthority(issue)
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

func issueTagProfile(item issues.Issue) []domain.TagRelevance {
	if len(item.TagScores) > 0 {
		return append([]domain.TagRelevance(nil), item.TagScores...)
	}

	if len(item.Tags) == 0 {
		return []domain.TagRelevance{}
	}

	profile := make([]domain.TagRelevance, 0, len(item.Tags))
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
		profile = append(profile, domain.TagRelevance{Tag: tag, Relevance: 1})
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

func topSharedTags(profile, issueTags []domain.TagRelevance, limit int) []string {
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

// personDetailTagData extracts tag names and embeddings from store tags and
// open issues for use in factor decomposition.
func personDetailTagData(openIssues []issues.Issue, storeTags []issues.Tag) ([]string, map[string][]float64) {
	seen := make(map[string]struct{})
	var tagNames []string

	for _, tag := range storeTags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			tagNames = append(tagNames, name)
		}
	}
	for _, issue := range openIssues {
		for _, ts := range issue.TagScores {
			name := strings.TrimSpace(ts.Tag)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				tagNames = append(tagNames, name)
			}
		}
	}
	slices.Sort(tagNames)

	tagEmbeddings := make(map[string][]float64, len(storeTags))
	for _, tag := range storeTags {
		name := strings.TrimSpace(tag.Name)
		if name != "" && len(tag.Embedding) > 0 {
			tagEmbeddings[name] = tag.Embedding
		}
	}

	return tagNames, tagEmbeddings
}
