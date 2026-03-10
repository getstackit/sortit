package issuemap

import (
	"cmp"
	"slices"
	"strings"

	"splat/internal/issues"
)

const defaultSearchLimit = 8

type SearchQuery struct {
	Raw  string         `json:"raw"`
	Tags []TagRelevance `json:"tags"`
}

type SearchResponse struct {
	Query         SearchQuery    `json:"query"`
	RelatedIssues []RelatedIssue `json:"relatedIssues"`
}

func SearchFromQueryWithTags(
	storeIssues []issues.Issue,
	storeTags []issues.Tag,
	queryRaw string,
	queryTags []issues.TagRelevance,
	queryEmbedding []float64,
	limit int,
) SearchResponse {
	queryRaw = strings.TrimSpace(queryRaw)
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	mapIssues, tagNames, issueEmbeddings, tagEmbeddings := runtimeMapInputs(storeIssues, storeTags)
	factorVectors := runtimeFactorVectors(mapIssues, tagNames, tagEmbeddings)

	querySummary := SearchQuery{
		Raw:  queryRaw,
		Tags: searchQueryTags(queryTags),
	}

	queryVector := append([]float64(nil), queryEmbedding...)
	if isZeroVector(queryVector) {
		queryVector = runtimeIssueEmbedding(issues.Issue{
			ID:        "query",
			Raw:       queryRaw,
			TagScores: querySummary.Tags,
		}, tagEmbeddings)
	}

	queryFactor := runtimeFactorVectors([]issues.Issue{{
		ID:        "query",
		Raw:       queryRaw,
		TagScores: querySummary.Tags,
	}}, tagNames, tagEmbeddings)["query"]

	related := make([]RelatedIssue, 0, len(storeIssues))
	for _, candidate := range storeIssues {
		candidateSummary := exploreIssueSummary(candidate)
		semantic := cosineSimilarity(queryVector, issueEmbeddings[candidate.ID])
		factor := cosineSimilarity(queryFactor, factorVectors[candidate.ID])
		combined := 0.6*semantic + 0.4*factor
		sharedTags := sharedRelevantTags(querySummary.Tags, candidateSummary.Tags, 3)

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

	return SearchResponse{
		Query:         querySummary,
		RelatedIssues: related,
	}
}

type RelatedTag struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Similarity  float64 `json:"similarity"`
}

func SearchTags(storeTags []issues.Tag, queryEmbedding []float64, limit int) []RelatedTag {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	related := make([]RelatedTag, 0, len(storeTags))
	rankedScores := make(map[string]float64, len(storeTags))
	for _, tag := range storeTags {
		if len(tag.Embedding) == 0 || len(queryEmbedding) == 0 {
			continue
		}
		sim := cosineSimilarity(queryEmbedding, tag.Embedding)
		rankedScores[tag.Name] = sim - genericBucketPenalty(tag.Name)
		related = append(related, RelatedTag{
			Name:        tag.Name,
			Description: tag.Description,
			Similarity:  round(sim, 2),
		})
	}

	slices.SortFunc(related, func(a, b RelatedTag) int {
		if diff := cmp.Compare(rankedScores[b.Name], rankedScores[a.Name]); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.Similarity, a.Similarity); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Name, b.Name)
	})

	if len(related) > limit {
		related = related[:limit]
	}

	return related
}

func searchQueryTags(tags []issues.TagRelevance) []TagRelevance {
	if len(tags) == 0 {
		return nil
	}

	queryTags := make([]TagRelevance, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Tag)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		queryTags = append(queryTags, TagRelevance{
			Tag:       name,
			Relevance: tag.Relevance,
		})
	}

	slices.SortFunc(queryTags, func(a, b TagRelevance) int {
		if diff := cmp.Compare(b.Relevance, a.Relevance); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tag, b.Tag)
	})

	return queryTags
}
