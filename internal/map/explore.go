package issuemap

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"splat/internal/issues"
)

const defaultExploreLimit = 8

type ExploreIssue struct {
	ID     string             `json:"id"`
	Raw    string             `json:"raw"`
	Status issues.IssueStatus `json:"status"`
	Tags   []TagRelevance     `json:"tags"`
}

type RelatedIssue struct {
	ID                 string             `json:"id"`
	Raw                string             `json:"raw"`
	Status             issues.IssueStatus `json:"status"`
	Tags               []TagRelevance     `json:"tags"`
	SemanticSimilarity float64            `json:"semanticSimilarity"`
	FactorSimilarity   float64            `json:"factorSimilarity"`
	CombinedSimilarity float64            `json:"combinedSimilarity"`
	Reason             string             `json:"reason"`
}

type Opportunity struct {
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Theme      string   `json:"theme"`
	IssueIDs   []string `json:"issueIds"`
	SharedTags []string `json:"sharedTags"`
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
}

type ExploreResponse struct {
	Issue         ExploreIssue   `json:"issue"`
	RelatedIssues []RelatedIssue `json:"relatedIssues"`
	Opportunities []Opportunity  `json:"opportunities"`
}

func ExploreFromIssuesWithTags(storeIssues []issues.Issue, storeTags []issues.Tag, targetID string, limit int) (ExploreResponse, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return ExploreResponse{}, issues.ErrNotFound
	}

	target, candidateSet, err := exploreInputIssues(storeIssues, targetID)
	if err != nil {
		return ExploreResponse{}, err
	}

	if limit <= 0 {
		limit = defaultExploreLimit
	}

	mapIssues, _, issueEmbeddings, tagEmbeddings := runtimeMapInputs(candidateSet, storeTags)
	factorVectors := runtimeFactorVectors(mapIssues, runtimeTagNames(candidateSet, storeTags), tagEmbeddings)

	targetSummary := exploreIssueSummary(target)
	targetEmbedding := issueEmbeddings[target.ID]
	targetFactor := factorVectors[target.ID]

	related := make([]RelatedIssue, 0, len(candidateSet)-1)
	for i := 1; i < len(candidateSet); i++ {
		candidate := candidateSet[i]
		candidateSummary := exploreIssueSummary(candidate)
		semantic := unitCosineSimilarity(targetEmbedding, issueEmbeddings[candidate.ID])
		factor := unitCosineSimilarity(targetFactor, factorVectors[candidate.ID])
		combined := 0.6*semantic + 0.4*factor
		sharedTags := sharedRelevantTags(targetSummary.Tags, candidateSummary.Tags, 3)

		related = append(related, RelatedIssue{
			ID:                 candidateSummary.ID,
			Raw:                candidateSummary.Raw,
			Status:             candidateSummary.Status,
			Tags:               candidateSummary.Tags,
			SemanticSimilarity: round(semantic, 2),
			FactorSimilarity:   round(factor, 2),
			CombinedSimilarity: round(combined, 2),
			Reason:             relatedIssueReason(sharedTags, semantic, factor),
		})
	}

	slices.SortFunc(related, func(a, b RelatedIssue) int {
		if diff := cmp.Compare(b.CombinedSimilarity, a.CombinedSimilarity); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.SemanticSimilarity, a.SemanticSimilarity); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.FactorSimilarity, a.FactorSimilarity); diff != 0 {
			return diff
		}
		return cmp.Compare(a.ID, b.ID)
	})

	if len(related) > limit {
		related = related[:limit]
	}

	return ExploreResponse{
		Issue:         targetSummary,
		RelatedIssues: related,
		Opportunities: buildExploreOpportunities(targetSummary, related),
	}, nil
}

func exploreInputIssues(storeIssues []issues.Issue, targetID string) (issues.Issue, []issues.Issue, error) {
	var (
		target issues.Issue
		found  bool
	)

	for _, item := range storeIssues {
		if item.ID == targetID {
			target = item
			found = true
			break
		}
	}

	if !found {
		return issues.Issue{}, nil, issues.ErrNotFound
	}

	candidateSet := make([]issues.Issue, 0, len(storeIssues))
	candidateSet = append(candidateSet, target)
	for _, item := range storeIssues {
		if item.ID == targetID || item.Status != issues.StatusOpen {
			continue
		}
		candidateSet = append(candidateSet, item)
	}

	return target, candidateSet, nil
}

func exploreIssueSummary(item issues.Issue) ExploreIssue {
	return ExploreIssue{
		ID:     item.ID,
		Raw:    item.Raw,
		Status: item.Status,
		Tags:   runtimeStoredTagRelevances(item),
	}
}

func runtimeFactorVectors(items []Issue, tags []string, tagEmbeddings map[string][]float64) map[string][]float64 {
	vectors := make(map[string][]float64, len(items))
	if len(items) == 0 || len(tags) == 0 {
		return vectors
	}

	tagIndex := make(map[string]int, len(tags))
	for i, tag := range tags {
		tagIndex[tag] = i
	}

	tagCov := buildTagCovariance(tags, tagEmbeddings)
	for _, item := range items {
		base := make([]float64, len(tags))
		for _, tag := range item.Tags {
			if index, ok := tagIndex[tag.Tag]; ok {
				base[index] = tag.Relevance
			}
		}

		vector := make([]float64, len(tags))
		for col := range vector {
			var sum float64
			for row, value := range base {
				sum += value * tagCov.At(row, col)
			}
			vector[col] = sum
		}
		if !isZeroVector(vector) {
			normalizeVector(vector)
		}
		vectors[item.ID] = vector
	}

	return vectors
}

func sharedRelevantTags(a, b []TagRelevance, limit int) []string {
	if limit <= 0 || len(a) == 0 || len(b) == 0 {
		return nil
	}

	left := make(map[string]float64, len(a))
	for _, item := range a {
		left[item.Tag] = item.Relevance
	}

	type scoredTag struct {
		tag   string
		score float64
	}

	var shared []scoredTag
	for _, item := range b {
		other, ok := left[item.Tag]
		if !ok {
			continue
		}
		score := minFloat(other, item.Relevance)
		if score < 0.2 {
			continue
		}
		shared = append(shared, scoredTag{
			tag:   item.Tag,
			score: score,
		})
	}

	slices.SortFunc(shared, func(a, b scoredTag) int {
		if diff := cmp.Compare(b.score, a.score); diff != 0 {
			return diff
		}
		return cmp.Compare(a.tag, b.tag)
	})

	if len(shared) > limit {
		shared = shared[:limit]
	}

	out := make([]string, len(shared))
	for i, item := range shared {
		out[i] = item.tag
	}
	return out
}

func relatedIssueReason(sharedTags []string, semantic, factor float64) string {
	if len(sharedTags) > 0 && factor >= semantic-0.05 {
		return "Shared factor relevance in " + strings.Join(sharedTags, ", ")
	}
	if semantic >= 0.75 {
		return "Semantically similar language suggests a shared root cause"
	}
	if factor >= 0.55 {
		return "Similar factor profile across related tags"
	}
	return "Related by blended semantic and factor similarity"
}

func buildExploreOpportunities(target ExploreIssue, related []RelatedIssue) []Opportunity {
	if len(related) == 0 {
		return []Opportunity{}
	}

	type opportunityGroup struct {
		themeKey   string
		theme      string
		reason     string
		sharedTags []string
		issues     []RelatedIssue
	}

	order := make([]string, 0, len(related))
	groups := make(map[string]*opportunityGroup, len(related))

	for _, item := range related {
		if item.CombinedSimilarity < 0.45 {
			continue
		}

		sharedTags := sharedRelevantTags(target.Tags, item.Tags, 2)
		themeKey := "semantic"
		theme := "shared root cause"
		reason := "These issues use closely related language and may share the same fix."
		if len(sharedTags) > 0 {
			themeKey = "tags:" + strings.Join(sharedTags, "|")
			theme = strings.Join(sharedTags, " + ")
			reason = "These issues share strong factor relevance in " + strings.Join(sharedTags, ", ") + "."
		}

		group, ok := groups[themeKey]
		if !ok {
			group = &opportunityGroup{
				themeKey:   themeKey,
				theme:      theme,
				reason:     reason,
				sharedTags: sharedTags,
			}
			groups[themeKey] = group
			order = append(order, themeKey)
		}
		group.issues = append(group.issues, item)
	}

	opportunities := make([]Opportunity, 0, len(groups))
	for _, key := range order {
		group := groups[key]
		if len(group.issues) == 0 {
			continue
		}

		issueIDs := make([]string, 0, len(group.issues)+1)
		issueIDs = append(issueIDs, target.ID)

		total := 0.0
		for _, item := range group.issues {
			issueIDs = append(issueIDs, item.ID)
			total += item.CombinedSimilarity
		}
		confidence := round(total/float64(len(group.issues)), 2)

		title := "Investigate a shared root cause"
		if len(group.sharedTags) > 0 {
			title = "Solve " + strings.Join(group.sharedTags, " + ") + " issues together"
		}

		summary := "A single fix may address " + target.ID + " and " + group.issues[0].ID + "."
		if len(group.issues) > 1 {
			summary = "A single fix may address " + target.ID + " plus " + pluralizeIssueCount(len(group.issues)) + "."
		}

		opportunities = append(opportunities, Opportunity{
			Title:      title,
			Summary:    summary,
			Theme:      group.theme,
			IssueIDs:   issueIDs,
			SharedTags: append([]string(nil), group.sharedTags...),
			Confidence: confidence,
			Reason:     group.reason,
		})
	}

	slices.SortFunc(opportunities, func(a, b Opportunity) int {
		if diff := cmp.Compare(b.Confidence, a.Confidence); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Title, b.Title)
	})

	return opportunities
}

func pluralizeIssueCount(count int) string {
	if count == 1 {
		return "1 related issue"
	}
	return strconv.Itoa(count) + " related issues"
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
