package issuemap

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"

	"splat/internal/domain"
	"splat/internal/issues"
	"splat/internal/scoring"
	"splat/internal/vectors"
)

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

type scoredResult struct {
	combined, semantic, factor, confidence float64
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
		limit = scoring.DefaultResultLimit
	}

	_, visible, _ := deriveRelationshipSemantics(storeIssues)
	mapIssues, tagNames, issueEmbeddings, tagEmbeddings := runtimeMapInputs(storeIssues, storeTags)
	decomp := ComputeFactorDecomposition(mapIssues, tagNames, issueEmbeddings, tagEmbeddings)

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

	// Fall back to legacy factor vectors when decomposition didn't produce per-issue vectors.
	useDecomp := decomp.Decomposed()
	var queryFactorEmb, queryResidualEmb []float64
	var factorVectors map[string][]float64
	if useDecomp {
		tagCov := buildTagCovariance(tagNames, tagEmbeddings)
		queryFactorEmb, queryResidualEmb = DecomposeEmbedding(queryVector, querySummary.Tags, tagNames, tagEmbeddings, tagCov)
	} else {
		factorVectors = runtimeFactorVectors(mapIssues, tagNames, tagEmbeddings)
	}

	queryFactor := factorVectors["query"]
	if !useDecomp && queryFactor == nil {
		queryFactor = runtimeFactorVectors([]issues.Issue{{
			ID:        "query",
			Raw:       queryRaw,
			TagScores: querySummary.Tags,
		}}, tagNames, tagEmbeddings)["query"]
	}

	queryLower := strings.ToLower(queryRaw)
	tagCorrelationBoost := queryMatchesTagNames(queryLower, tagNames)
	tagSpecificity := buildTagSpecificityMap(storeTags)
	genericQuery := queryMatchesGenericTag(queryLower, tagSpecificity)
	now := time.Now().UTC()

	// When query matches a tag name, nudge factor weight up.
	searchDecomp := decomp
	if tagCorrelationBoost && useDecomp {
		searchDecomp.FactorWeight = clamp(decomp.FactorWeight+scoring.TagCorrelationFactorNudge, scoring.MinFactorWeight, scoring.MaxFactorWeight)
		searchDecomp.ResidualWeight = 1 - searchDecomp.FactorWeight
	}

	related := make([]RelatedIssue, 0, len(storeIssues))
	scores := make([]scoredResult, 0, len(storeIssues))
	for _, candidate := range storeIssues {
		if _, ok := visible[candidate.ID]; !ok {
			continue
		}
		candidateSummary := exploreIssueSummary(candidate)
		semanticSim := vectors.CosineSimilarity(queryVector, issueEmbeddings[candidate.ID])

		var factorSim, residualSim, blended float64
		if useDecomp && len(decomp.FactorEmbedding(candidate.ID)) > 0 {
			factorSim, _, blended = BlendFromDecomposition(
				searchDecomp, queryFactorEmb, queryResidualEmb,
				decomp.FactorEmbedding(candidate.ID), decomp.ResidualEmbedding(candidate.ID),
			)
		} else {
			residualSim = semanticSim
			factorSim = vectors.CosineSimilarity(queryFactor, factorVectors[candidate.ID])
			if tagCorrelationBoost {
				blended = scoring.TagCorrelationSemantic*residualSim + scoring.TagCorrelationFactor*factorSim
			} else {
				blended = scoring.SemanticWeight*residualSim + scoring.FactorWeight*factorSim
			}
		}

		combined := blended
		// Note: the DB query also applies recency decay (90-day half-life) to rank
		// the retrieval window. This app-side freshness weight fine-tunes within
		// the semantic/factor blend on the already-retrieved candidates.
		combined *= issues.IssueFreshnessWeight(candidate, now)
		combined *= 1 + scoring.SearchVelocityBoost*issueVelocityScore(candidate)
		combined += issues.IssueAuthority(candidate) * scoring.AuthorityConsumerWt
		combined -= issueSpecificityPenalty(candidateSummary.Tags, tagSpecificity)
		if genericQuery {
			combined += specificCooccurrenceBoost(candidateSummary.Tags, tagSpecificity)
		}
		sharedTags := sharedRelevantTags(querySummary.Tags, candidateSummary.Tags, scoring.SharedTagsLimit)

		scores = append(scores, scoredResult{
			combined:   combined,
			semantic:   semanticSim,
			factor:     factorSim,
			confidence: issues.ComputeContentConfidence(candidate.Raw),
		})

		related = append(related, RelatedIssue{
			ID:                 candidateSummary.ID,
			Raw:                candidateSummary.Raw,
			Status:             candidateSummary.Status,
			Tags:               candidateSummary.Tags,
			SemanticSimilarity: round(semanticSim),
			FactorSimilarity:   round(factorSim),
			CombinedSimilarity: round(combined),
			Reason:             relatedIssueReason(sharedTags, semanticSim, factorSim),
		})
	}

	sortSearchResults(related, storeIssues, cfg.sortBy, scores)

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
// scores is a parallel slice to related — scores[i] holds the raw scores for related[i].
func sortSearchResults(
	related []RelatedIssue,
	storeIssues []issues.Issue,
	sortBy string,
	scores []scoredResult,
) {
	switch sortBy {
	case "created_at":
		issueIndex := make(map[string]issues.Issue, len(storeIssues))
		for _, issue := range storeIssues {
			issueIndex[issue.ID] = issue
		}
		sortBoth(related, scores, func(ai, bi int) int {
			aTime := issueIndex[related[ai].ID].CreatedAt
			bTime := issueIndex[related[bi].ID].CreatedAt
			if diff := bTime.Compare(aTime); diff != 0 {
				return diff
			}
			return cmp.Compare(related[ai].ID, related[bi].ID)
		})
	default: // "relevance"
		sortBoth(related, scores, func(ai, bi int) int {
			a, b := scores[ai], scores[bi]
			if math.Abs(b.combined-a.combined) > scoring.ContentConfidenceTieWindow {
				return cmp.Compare(b.combined, a.combined)
			}
			if diff := cmp.Compare(b.confidence, a.confidence); diff != 0 {
				return diff
			}
			if diff := cmp.Compare(b.combined, a.combined); diff != 0 {
				return diff
			}
			if diff := cmp.Compare(b.semantic, a.semantic); diff != 0 {
				return diff
			}
			if diff := cmp.Compare(b.factor, a.factor); diff != 0 {
				return diff
			}
			return cmp.Compare(related[ai].ID, related[bi].ID)
		})
	}
}

// sortBoth sorts related and scores in tandem using an index-based comparator.
func sortBoth(related []RelatedIssue, scores []scoredResult, cmpFn func(ai, bi int) int) {
	indices := make([]int, len(related))
	for i := range indices {
		indices[i] = i
	}
	slices.SortFunc(indices, cmpFn)

	sortedRelated := make([]RelatedIssue, len(related))
	sortedScores := make([]scoredResult, len(scores))
	for i, idx := range indices {
		sortedRelated[i] = related[idx]
		sortedScores[i] = scores[idx]
	}
	copy(related, sortedRelated)
	copy(scores, sortedScores)
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

// issueSpecificityPenalty penalizes issues whose top tags are all generic
// (low specificity), so that issues with specific tags rank above generic ones.
func issueSpecificityPenalty(tags []TagRelevance, tagSpecificity map[string]*float64) float64 {
	if len(tags) == 0 {
		return 0
	}
	limit := min(scoring.SpecificityPenaltyTopN, len(tags))
	var totalPenalty float64
	for _, tag := range tags[:limit] {
		totalPenalty += specificityPenalty(tagSpecificity[tag.Tag])
	}
	return totalPenalty / float64(limit)
}

// queryMatchesGenericTag returns true if any query word exactly matches a
// tag with low specificity (< 0.5), indicating the user is filtering by a
// broad category.
func queryMatchesGenericTag(queryLower string, tagSpecificity map[string]*float64) bool {
	if queryLower == "" {
		return false
	}
	for w := range strings.FieldsSeq(queryLower) {
		normalized := domain.NormalizeTagName(w)
		if p, ok := tagSpecificity[normalized]; ok {
			s := scoring.GenericTagThreshold
			if p != nil {
				s = *p
			}
			if s < scoring.GenericTagThreshold {
				return true
			}
		}
	}
	return false
}

// specificCooccurrenceBoost returns a small positive boost for issues that
// carry specific tags (specificity >= 0.5) alongside generic ones. When a user
// searches by a generic tag, issues with co-occurring specific tags should
// rank above issues with only generic tags.
func specificCooccurrenceBoost(tags []TagRelevance, tagSpecificity map[string]*float64) float64 {
	boost := 0.0
	for _, tag := range tags {
		if tag.Relevance <= scoring.CooccurrenceRelevanceMin {
			continue
		}
		s := scoring.GenericTagThreshold
		if p, ok := tagSpecificity[tag.Tag]; ok && p != nil {
			s = *p
		}
		if s >= scoring.GenericTagThreshold {
			boost += scoring.CooccurrenceBoostPerTag
			if boost >= scoring.CooccurrenceBoostMax {
				return scoring.CooccurrenceBoostMax
			}
		}
	}
	return boost
}

// buildTagSpecificityMap builds a lookup from tag name to its specificity score pointer.
func buildTagSpecificityMap(tags []issues.Tag) map[string]*float64 {
	m := make(map[string]*float64, len(tags))
	for i := range tags {
		m[tags[i].Name] = tags[i].Specificity
	}
	return m
}

type RelatedTag struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Similarity  float64 `json:"similarity"`
}

func SearchTags(storeTags []issues.Tag, queryEmbedding []float64, limit int) []RelatedTag {
	if limit <= 0 {
		limit = scoring.DefaultResultLimit
	}

	related := make([]RelatedTag, 0, len(storeTags))
	rankedScores := make(map[string]float64, len(storeTags))
	for _, tag := range storeTags {
		if len(tag.Embedding) == 0 || len(queryEmbedding) == 0 {
			continue
		}
		sim := vectors.CosineSimilarity(queryEmbedding, tag.Embedding)
		rankedScores[tag.Name] = sim - specificityPenalty(tag.Specificity)
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
