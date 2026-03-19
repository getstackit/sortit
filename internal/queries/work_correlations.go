package queries

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strings"

	"splat/internal/issues"
	"splat/internal/scoring"
	"splat/internal/services"
	"splat/internal/vectors"
)

type PersonCorrelation struct {
	PersonA           string         `json:"personA"`
	PersonB           string         `json:"personB"`
	CombinedScore     float64        `json:"combinedScore"`
	SemanticScore     float64        `json:"semanticScore"`
	FactorScore       float64        `json:"factorScore"`
	SharedTags        []string       `json:"sharedTags"`
	PersonAIssueCount int            `json:"personAIssueCount"`
	PersonBIssueCount int            `json:"personBIssueCount"`
	PersonAProfile    []TagRelevance `json:"personAProfile"`
	PersonBProfile    []TagRelevance `json:"personBProfile"`
}

type WorkCorrelationsResult struct {
	Correlations []PersonCorrelation `json:"correlations"`
}

type WorkCorrelationsHandler struct {
	Store   issues.Store
	Catalog *services.CatalogService
}

func (h WorkCorrelationsHandler) Handle(ctx context.Context, filter IssueStatusFilter) (WorkCorrelationsResult, error) {
	tagSpecificity := loadTagSpecificityMap(ctx, h.Catalog)

	if store, ok := h.Store.(peopleAnalyticsLister); ok {
		items, err := store.ListPeopleAnalytics(ctx, issues.ListOptions{
			Status: issueStatusFromFilter(filter),
		})
		if err != nil {
			return WorkCorrelationsResult{}, err
		}
		return buildWorkCorrelations(items, IssueStatusFilterAll, tagSpecificity), nil
	}

	if store, ok := h.Store.(filteredIssueLister); ok {
		items, err := store.ListFiltered(ctx, issues.ListOptions{
			Status: issueStatusFromFilter(filter),
		})
		if err != nil {
			return WorkCorrelationsResult{}, err
		}
		return buildWorkCorrelations(peopleAnalyticsIssuesFromIssues(items), IssueStatusFilterAll, tagSpecificity), nil
	}

	if h.Store != nil {
		allIssues, err := h.Store.List(ctx)
		if err != nil {
			return WorkCorrelationsResult{}, err
		}
		return buildWorkCorrelations(peopleAnalyticsIssuesFromIssues(allIssues), filter, tagSpecificity), nil
	}

	return WorkCorrelationsResult{Correlations: []PersonCorrelation{}}, nil
}

func buildWorkCorrelations(allIssues []issues.PeopleAnalyticsIssue, filter IssueStatusFilter, tagSpecificity map[string]*float64) WorkCorrelationsResult {
	allIssues = filterPeopleAnalyticsByStatus(allIssues, filter)

	// Group issues by assignee
	byPerson := make(map[string][]issues.PeopleAnalyticsIssue)
	for _, issue := range allIssues {
		if issue.AssignedTo == "" {
			continue
		}
		key := strings.ToLower(issue.AssignedTo)
		byPerson[key] = append(byPerson[key], issue)
	}

	if len(byPerson) < 2 {
		return WorkCorrelationsResult{Correlations: []PersonCorrelation{}}
	}

	type personData struct {
		name       string
		issues     []issues.PeopleAnalyticsIssue
		tagProfile []TagRelevance
		embedding  []float64
	}

	people := make([]personData, 0, len(byPerson))
	for _, personIssues := range byPerson {
		pd := personData{
			name:       personIssues[0].AssignedTo,
			issues:     personIssues,
			tagProfile: meanTagProfile(personIssues, tagSpecificity),
			embedding:  meanEmbedding(personIssues),
		}
		people = append(people, pd)
	}

	slices.SortStableFunc(people, func(a, b personData) int {
		return cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})

	var correlations []PersonCorrelation
	for i := 0; i < len(people); i++ {
		for j := i + 1; j < len(people); j++ {
			a, b := people[i], people[j]

			semanticScore := vectors.CosineSimilarity(a.embedding, b.embedding)
			factorScore := tagProfileSimilarity(a.tagProfile, b.tagProfile)
			combined := scoring.CorrelationSemanticWeight*semanticScore + scoring.CorrelationFactorWeight*factorScore

			correlations = append(correlations, PersonCorrelation{
				PersonA:           a.name,
				PersonB:           b.name,
				CombinedScore:     roundTo2(combined),
				SemanticScore:     roundTo2(semanticScore),
				FactorScore:       roundTo2(factorScore),
				SharedTags:        sharedTags(a.tagProfile, b.tagProfile),
				PersonAIssueCount: len(a.issues),
				PersonBIssueCount: len(b.issues),
				PersonAProfile:    a.tagProfile,
				PersonBProfile:    b.tagProfile,
			})
		}
	}

	slices.SortStableFunc(correlations, func(a, b PersonCorrelation) int {
		return cmp.Compare(b.CombinedScore, a.CombinedScore)
	})

	return WorkCorrelationsResult{Correlations: correlations}
}

func meanTagProfile(matched []issues.PeopleAnalyticsIssue, tagSpecificity map[string]*float64) []TagRelevance {
	if len(matched) == 0 {
		return []TagRelevance{}
	}

	sums := make(map[string]float64)
	counts := make(map[string]int)
	for _, issue := range matched {
		for _, ts := range issue.TagScores {
			weighted := ts.Relevance * specificityWeight(tagSpecificity[ts.Tag])
			sums[ts.Tag] += weighted
			counts[ts.Tag]++
		}
	}

	profile := make([]TagRelevance, 0, len(sums))
	for tag, sum := range sums {
		profile = append(profile, TagRelevance{
			Tag:       tag,
			Relevance: roundTo2(sum / float64(len(matched))),
		})
	}

	slices.SortStableFunc(profile, func(a, b TagRelevance) int {
		if c := cmp.Compare(b.Relevance, a.Relevance); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})

	return profile
}

func meanEmbedding(matched []issues.PeopleAnalyticsIssue) []float64 {
	if len(matched) == 0 {
		return nil
	}

	var dim int
	for _, issue := range matched {
		if len(issue.Embedding) > 0 {
			dim = len(issue.Embedding)
			break
		}
	}
	if dim == 0 {
		return nil
	}

	mean := make([]float64, dim)
	count := 0
	for _, issue := range matched {
		if len(issue.Embedding) != dim {
			continue
		}
		for k, v := range issue.Embedding {
			mean[k] += v
		}
		count++
	}

	if count == 0 {
		return nil
	}

	var mag float64
	for k := range mean {
		mean[k] /= float64(count)
		mag += mean[k] * mean[k]
	}

	if mag > 0 {
		mag = math.Sqrt(mag)
		for k := range mean {
			mean[k] /= mag
		}
	}

	return mean
}

func tagProfileSimilarity(a, b []TagRelevance) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	tags := make(map[string]struct{})
	aMap := make(map[string]float64, len(a))
	bMap := make(map[string]float64, len(b))
	for _, tr := range a {
		aMap[tr.Tag] = tr.Relevance
		tags[tr.Tag] = struct{}{}
	}
	for _, tr := range b {
		bMap[tr.Tag] = tr.Relevance
		tags[tr.Tag] = struct{}{}
	}

	var dot, magA, magB float64
	for tag := range tags {
		av, bv := aMap[tag], bMap[tag]
		dot += av * bv
		magA += av * av
		magB += bv * bv
	}

	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

func sharedTags(a, b []TagRelevance) []string {
	bSet := make(map[string]struct{}, len(b))
	for _, tr := range b {
		bSet[tr.Tag] = struct{}{}
	}

	var shared []string
	seen := make(map[string]struct{})
	for _, tr := range a {
		if _, ok := bSet[tr.Tag]; ok {
			if _, dup := seen[tr.Tag]; !dup {
				shared = append(shared, tr.Tag)
				seen[tr.Tag] = struct{}{}
			}
		}
	}

	if shared == nil {
		return []string{}
	}
	return shared
}

func peopleAnalyticsIssuesFromIssues(items []issues.Issue) []issues.PeopleAnalyticsIssue {
	if len(items) == 0 {
		return nil
	}

	out := make([]issues.PeopleAnalyticsIssue, 0, len(items))
	for _, item := range items {
		out = append(out, issues.PeopleAnalyticsIssue{
			Status:     item.Status,
			AssignedTo: item.AssignedTo,
			TagScores:  append([]issues.TagRelevance(nil), item.TagScores...),
			Embedding:  append([]float64(nil), item.Embedding...),
		})
	}
	return out
}

func filterPeopleAnalyticsByStatus(items []issues.PeopleAnalyticsIssue, filter IssueStatusFilter) []issues.PeopleAnalyticsIssue {
	if filter == "" {
		filter = IssueStatusFilterOpen
	}
	if filter == IssueStatusFilterAll {
		return items
	}

	filtered := make([]issues.PeopleAnalyticsIssue, 0, len(items))
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

func roundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}

// specificityWeight returns the specificity value for weighting, defaulting
// to GenericTagThreshold when the score is nil (unscored).
func specificityWeight(s *float64) float64 {
	if s == nil {
		return scoring.GenericTagThreshold
	}
	return *s
}

// loadTagSpecificityMap loads stored tags from the catalog and builds a map
// from tag name to specificity pointer. Returns nil if the catalog is
// unavailable or loading fails (callers treat nil map entries as default).
func loadTagSpecificityMap(ctx context.Context, catalog *services.CatalogService) map[string]*float64 {
	if catalog == nil {
		return nil
	}
	tags, err := catalog.StoredTags(ctx)
	if err != nil {
		return nil
	}
	m := make(map[string]*float64, len(tags))
	for i := range tags {
		m[tags[i].Name] = tags[i].Specificity
	}
	return m
}
