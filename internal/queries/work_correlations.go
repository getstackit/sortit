package queries

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strings"

	"splat/internal/issues"
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
	Store issues.Store
}

func (h WorkCorrelationsHandler) Handle(ctx context.Context, filter IssueStatusFilter) (WorkCorrelationsResult, error) {
	if store, ok := h.Store.(filteredIssueLister); ok {
		items, err := store.ListFiltered(ctx, issues.ListOptions{
			Status: issueStatusFromFilter(filter),
		})
		if err != nil {
			return WorkCorrelationsResult{}, err
		}
		return buildWorkCorrelations(items, IssueStatusFilterAll), nil
	}

	if h.Store != nil {
		allIssues, err := h.Store.List(ctx)
		if err != nil {
			return WorkCorrelationsResult{}, err
		}
		return buildWorkCorrelations(allIssues, filter), nil
	}

	return WorkCorrelationsResult{Correlations: []PersonCorrelation{}}, nil
}

func buildWorkCorrelations(allIssues []issues.Issue, filter IssueStatusFilter) WorkCorrelationsResult {
	allIssues = FilterIssuesByStatus(allIssues, filter)

	// Group issues by assignee
	byPerson := make(map[string][]issues.Issue)
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
		issues     []issues.Issue
		tagProfile []TagRelevance
		embedding  []float64
	}

	people := make([]personData, 0, len(byPerson))
	for _, personIssues := range byPerson {
		pd := personData{
			name:       personIssues[0].AssignedTo,
			issues:     personIssues,
			tagProfile: meanTagProfile(personIssues),
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
			combined := 0.6*semanticScore + 0.4*factorScore

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

func meanTagProfile(matched []issues.Issue) []TagRelevance {
	if len(matched) == 0 {
		return []TagRelevance{}
	}

	sums := make(map[string]float64)
	counts := make(map[string]int)
	for _, issue := range matched {
		for _, ts := range issue.TagScores {
			sums[ts.Tag] += ts.Relevance
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

func meanEmbedding(matched []issues.Issue) []float64 {
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

func roundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}
