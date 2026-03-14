package issuemap

import (
	"cmp"
	"slices"
	"strings"

	"splat/internal/domain"
	"splat/internal/issues"
	"splat/internal/vectors"
)

const defaultSearchLimit = 8

// SearchOptions holds optional search parameters.
type SearchOptions struct {
	Query      string
	QueryTags  []issues.TagRelevance
	QueryEmbed []float64
	Limit      int
	Offset     int
	SortBy     string // "relevance" (default), "created_at"
}

// SearchOption is a functional option for search functions.
type SearchOption func(*searchConfig)

type searchConfig struct {
	offset int
	sortBy string
}

func WithOffset(offset int) SearchOption {
	return func(c *searchConfig) {
		if offset > 0 {
			c.offset = offset
		}
	}
}

func WithSortBy(sortBy string) SearchOption {
	return func(c *searchConfig) {
		c.sortBy = sortBy
	}
}

func applySearchOptions(opts []SearchOption) searchConfig {
	cfg := searchConfig{sortBy: "relevance"}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

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
	opts ...SearchOption,
) SearchResponse {
	cfg := applySearchOptions(opts)
	queryRaw = strings.TrimSpace(queryRaw)
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	_, visible, _ := deriveRelationshipSemantics(storeIssues)
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

	queryLower := strings.ToLower(queryRaw)
	tagCorrelationBoost := queryMatchesTagNames(queryLower, tagNames)

	related := make([]RelatedIssue, 0, len(storeIssues))
	for _, candidate := range storeIssues {
		if _, ok := visible[candidate.ID]; !ok {
			continue
		}
		candidateSummary := exploreIssueSummary(candidate)
		semantic := vectors.CosineSimilarity(queryVector, issueEmbeddings[candidate.ID])
		factor := vectors.CosineSimilarity(queryFactor, factorVectors[candidate.ID])
		textMatch := textMatchScore(queryLower, candidate.Raw)
		combined := blendScores(semantic, factor, textMatch, tagCorrelationBoost)
		combined -= issueSpecificityPenalty(candidateSummary.Tags)
		sharedTags := sharedRelevantTags(querySummary.Tags, candidateSummary.Tags, 3)

		related = append(related, RelatedIssue{
			ID:                 candidateSummary.ID,
			Raw:                candidateSummary.Raw,
			Status:             candidateSummary.Status,
			Tags:               candidateSummary.Tags,
			SemanticSimilarity: round(semantic),
			FactorSimilarity:   round(factor),
			CombinedSimilarity: round(combined),
			Reason:             relatedIssueReason(sharedTags, semantic, factor),
		})
	}

	sortSearchResults(related, storeIssues, cfg.sortBy)

	// Apply offset then limit.
	if cfg.offset > 0 && cfg.offset < len(related) {
		related = related[cfg.offset:]
	} else if cfg.offset >= len(related) {
		related = nil
	}
	if len(related) > limit {
		related = related[:limit]
	}

	return SearchResponse{
		Query:         querySummary,
		RelatedIssues: related,
	}
}

// sortSearchResults sorts results by the given sort key.
func sortSearchResults(related []RelatedIssue, storeIssues []issues.Issue, sortBy string) {
	switch sortBy {
	case "created_at":
		issueIndex := make(map[string]issues.Issue, len(storeIssues))
		for _, issue := range storeIssues {
			issueIndex[issue.ID] = issue
		}
		slices.SortFunc(related, func(a, b RelatedIssue) int {
			aTime := issueIndex[a.ID].CreatedAt
			bTime := issueIndex[b.ID].CreatedAt
			if diff := bTime.Compare(aTime); diff != 0 {
				return diff
			}
			return cmp.Compare(a.ID, b.ID)
		})
	default: // "relevance"
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
	}
}

// queryMatchesTagNames returns true if any query word exactly matches a known
// tag name, indicating the user is searching by tag-space concepts.
func queryMatchesTagNames(queryLower string, tagNames []string) bool {
	if queryLower == "" || len(tagNames) == 0 {
		return false
	}
	words := make(map[string]struct{})
	for w := range strings.FieldsSeq(queryLower) {
		words[domain.NormalizeTagName(w)] = struct{}{}
	}
	for _, tag := range tagNames {
		if _, ok := words[domain.NormalizeTagName(tag)]; ok {
			return true
		}
	}
	return false
}

// blendScores combines the three similarity signals. When the query matches
// known tag names, the factor weight is boosted so tag-correlated issues
// rank higher.
func blendScores(semantic, factor, textMatch float64, tagCorrelation bool) float64 {
	if tagCorrelation {
		// Boost factor weight: 0.4 semantic, 0.4 factor, 0.2 text
		return 0.4*semantic + 0.4*factor + 0.2*textMatch
	}
	return 0.5*semantic + 0.3*factor + 0.2*textMatch
}

// issueSpecificityPenalty penalizes issues whose top tags are all generic
// bucket tags, so that issues with specific tags rank above generic ones.
func issueSpecificityPenalty(tags []TagRelevance) float64 {
	if len(tags) == 0 {
		return 0
	}
	// Check up to the top 3 tags by relevance (they come pre-sorted).
	limit := min(3, len(tags))
	genericCount := 0
	for _, tag := range tags[:limit] {
		if genericBucketPenalty(tag.Tag) > 0 {
			genericCount++
		}
	}
	if genericCount == 0 {
		return 0
	}
	// Small penalty proportional to how generic the top tags are.
	return 0.02 * float64(genericCount) / float64(limit)
}

// textMatchScore computes a lightweight text-match signal between the query
// and issue text, returning a value in [0, 1]. It uses substring matching
// on the full query and individual words for broad coverage.
func textMatchScore(queryLower string, issueRaw string) float64 {
	if queryLower == "" || issueRaw == "" {
		return 0
	}
	issueLower := strings.ToLower(issueRaw)

	// Full substring match is the strongest signal.
	if strings.Contains(issueLower, queryLower) {
		return 1.0
	}

	// Word-level overlap: fraction of query words found in the issue text.
	words := strings.Fields(queryLower)
	if len(words) == 0 {
		return 0
	}
	matches := 0
	for _, word := range words {
		if len(word) < 2 {
			continue
		}
		if strings.Contains(issueLower, word) {
			matches++
		}
	}
	return float64(matches) / float64(len(words))
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
		sim := vectors.CosineSimilarity(queryEmbedding, tag.Embedding)
		rankedScores[tag.Name] = sim - genericBucketPenalty(tag.Name)
		related = append(related, RelatedTag{
			Name:        tag.Name,
			Description: tag.Description,
			Similarity:  round(sim),
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
