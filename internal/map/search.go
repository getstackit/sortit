package issuemap

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issueanalytics"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/vectors"
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
	// regionTarget, when non-empty, identifies the tag whose region the
	// query is asking about (typically the tag name parsed out of the
	// query string). Candidates with relevance >= 0.4 on this tag get a
	// small boost.
	regionTarget string
	// antiCorrelators are tags that the regionTarget structurally
	// dis-occurs with, sourced from the co-occurrence projection. When
	// a candidate has any of these tags at strong relevance, the score
	// is penalized.
	antiCorrelators map[string]float64
	// corpusMeans, when non-nil, are the revision-cached full-corpus
	// embedding means used for centering. Required when storeIssues is a
	// retrieved subset, whose own mean would erase the query signal.
	corpusMeans *issuemath.CorpusMeans
	// factorWeightOverride, when non-nil, replaces the data-driven factor
	// share of the similarity blend (and the tag-correlation nudge) with a
	// fixed value. Evaluation hook for the matheval sweep; production
	// callers leave it unset.
	factorWeightOverride *float64
	// ridgeLambda, when non-nil, switches the similarity model from the
	// rank-1 factor projection to the full-rank anchored-ridge blend
	// (tag-space), with this value as the unscored-tag penalty. The value is
	// expected to be GCV-derived upstream and cached by corpus revision —
	// search does not run the selection itself. Phase 3b A/B opt-in;
	// production default leaves it unset (rank-1).
	ridgeLambda *float64
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

// WithRegionTarget marks `tag` as the query's target region. The search
// applies a small boost (scoring.RegionMatchBoost) to candidates that
// are members of the region (relevance >= MembershipFloor).
func WithRegionTarget(tag string) SearchOption {
	return func(c *searchConfig) {
		c.regionTarget = strings.TrimSpace(tag)
	}
}

// WithAntiCorrelators supplies the set of tags that the target region
// dis-occurs with along with their per-pair implicit-negative weight.
// Candidates carrying any of these tags at strong relevance get
// penalized by weight * scoring.RegionAntiCorrelationPenalty.
func WithAntiCorrelators(tags map[string]float64) SearchOption {
	return func(c *searchConfig) {
		if len(tags) == 0 {
			return
		}
		normalized := make(map[string]float64, len(tags))
		for tag, weight := range tags {
			name := domain.NormalizeTagName(tag)
			if name == "" || weight <= 0 {
				continue
			}
			normalized[name] = weight
		}
		if len(normalized) == 0 {
			return
		}
		c.antiCorrelators = normalized
	}
}

// WithFactorWeightOverride forces the factor share of the similarity blend
// to a fixed value in [0, 1] instead of the data-driven decomposition weight,
// disabling the tag-correlation nudge. This is an evaluation hook for the
// matheval blend-weight sweep; production search paths do not set it.
func WithFactorWeightOverride(factorWeight float64) SearchOption {
	return func(c *searchConfig) {
		w := min(max(factorWeight, 0), 1)
		c.factorWeightOverride = &w
	}
}

// WithRidgeSimilarity switches the similarity model to the full-rank
// anchored-ridge blend (tag-space cos(f_A,f_B)), using lambda as the
// unscored-tag penalty. lambda should be the GCV-derived value from
// issuemath.SelectRidgeLambdaGCV, computed over the full corpus and cached by
// revision — search does not run the selection. Non-positive lambda is
// ignored (keeps the rank-1 model). When the corpus is too small for the
// ridge decomposition, search falls back to rank-1 automatically.
func WithRidgeSimilarity(lambda float64) SearchOption {
	return func(c *searchConfig) {
		if lambda <= 0 {
			return
		}
		c.ridgeLambda = &lambda
	}
}

// WithCorpusMeans supplies revision-cached full-corpus embedding means for
// centering. Without this option the means are derived from the supplied
// issue set, which is only correct when that set is the full corpus.
func WithCorpusMeans(means issuemath.CorpusMeans) SearchOption {
	return func(c *searchConfig) {
		if means.IsZero() {
			return
		}
		c.corpusMeans = &means
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
	corpus := runtimeMapInputsWithMeans(storeIssues, storeTags, cfg.corpusMeans)
	mapIssues, tagNames := corpus.issues, corpus.tagNames
	issueEmbeddings, tagEmbeddings := corpus.issueEmbeddings, corpus.tagEmbeddings
	decomp := issuemath.ComputeFactorDecomposition(mapIssues, tagNames, issueEmbeddings, tagEmbeddings)

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
	// Center the query with the corpus means — never with statistics derived
	// from the query or the candidate set itself.
	queryVector = issuemath.CenterVector(queryVector, corpus.means.Issue)

	// Fall back to legacy factor vectors when decomposition didn't produce per-issue vectors.
	useDecomp := decomp.Decomposed()
	var queryDecomposed issuemath.DecomposedEmbedding
	var factorVectors map[string][]float64
	if useDecomp {
		tagCov := issuemath.BuildTagCovariance(tagNames, tagEmbeddings)
		queryDecomposed = issuemath.DecomposeEmbedding(queryVector, querySummary.Tags, tagNames, tagEmbeddings, tagCov)
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

	// adjustFactorWeight applies the tag-correlation nudge (when the query
	// names a tag) and the evaluation override to a model's factor share.
	// Shared by the rank-1 and ridge models so both react identically.
	adjustFactorWeight := func(fw float64) (factor, residual float64) {
		if tagCorrelationBoost {
			fw = min(max(fw+scoring.TagCorrelationFactorNudge, scoring.MinFactorWeight), scoring.MaxFactorWeight)
		}
		if cfg.factorWeightOverride != nil {
			fw = *cfg.factorWeightOverride
		}
		return fw, 1 - fw
	}

	// When query matches a tag name, nudge factor weight up.
	searchDecomp := decomp
	if useDecomp {
		searchDecomp.FactorWeight, searchDecomp.ResidualWeight = adjustFactorWeight(decomp.FactorWeight)
	}

	// Ridge similarity model (Phase 3b, opt-in via WithRidgeSimilarity). Only
	// engaged when the corpus is large enough for both decompositions; falls
	// back to rank-1 otherwise.
	useRidge := cfg.ridgeLambda != nil && useDecomp
	var ridgeDecomp issuemath.RidgeDecomposition
	var queryRidge issuemath.RidgeVectors
	if useRidge {
		ridgeDecomp = issuemath.ComputeRidgeDecomposition(mapIssues, tagNames, issueEmbeddings, tagEmbeddings,
			scoring.RidgeAnchorLambdaScored, *cfg.ridgeLambda)
		if ridgeDecomp.Decomposed() {
			ridgeDecomp.FactorWeight, ridgeDecomp.ResidualWeight = adjustFactorWeight(ridgeDecomp.FactorWeight)
			queryRidge = issuemath.DecomposeRidgeEmbedding(queryVector, querySummary.Tags, tagNames, tagEmbeddings,
				scoring.RidgeAnchorLambdaScored, *cfg.ridgeLambda)
		} else {
			useRidge = false
		}
	}

	related := make([]RelatedIssue, 0, len(storeIssues))
	scores := make([]scoredResult, 0, len(storeIssues))
	for _, candidate := range storeIssues {
		if _, ok := visible[candidate.ID]; !ok {
			continue
		}
		candidateSummary := exploreIssueSummary(candidate)
		semanticSim := vectors.CosineSimilarity(queryVector, issueEmbeddings[candidate.ID])

		// Missing-from-decomposition candidates (no persisted embedding, or a
		// dimension mismatch after an embedding-model change) score as pure
		// semantic similarity at full weight: blending a zero factor side at
		// w_F would deflate them relative to decomposed candidates in the
		// same ranked list.
		var factorSim, blended float64
		switch {
		case useRidge:
			if cv, ok := ridgeDecomp.VectorsFor(candidate.ID); ok {
				factorSim, _, blended = issuemath.RidgeBlend(ridgeDecomp, queryRidge, cv, issuemath.RidgeTagSpace)
			} else {
				blended = semanticSim
			}
		case useDecomp:
			if candidateDecomposed, ok := decomp.DecomposedFor(candidate.ID); ok {
				factorSim, _, blended = issuemath.BlendFromDecomposition(
					searchDecomp, queryDecomposed, candidateDecomposed,
				)
			} else {
				blended = semanticSim
			}
		default:
			factorSim = vectors.CosineSimilarity(queryFactor, factorVectors[candidate.ID])
			factorShare := scoring.FactorWeight
			if tagCorrelationBoost {
				factorShare = scoring.TagCorrelationFactor
			}
			if cfg.factorWeightOverride != nil {
				factorShare = *cfg.factorWeightOverride
			}
			blended = (1-factorShare)*semanticSim + factorShare*factorSim
		}

		// Composition rule: query-relative evidence is additive, candidate
		// quality modulates multiplicatively.
		//
		// The similarity blend and the query-conditional boosts/penalties
		// all assert how well this candidate matches *this query*, so they
		// add, then clamp at zero — no relevance evidence means no score.
		evidence := blended
		if genericQuery {
			evidence += specificCooccurrenceBoost(candidateSummary.Tags, tagSpecificity)
		}
		if cfg.regionTarget != "" && candidateInRegion(candidateSummary.Tags, cfg.regionTarget) {
			evidence += scoring.RegionMatchBoost
		}
		if len(cfg.antiCorrelators) > 0 {
			evidence -= candidateAntiCorrelationPenalty(candidateSummary.Tags, cfg.antiCorrelators)
		}
		evidence = max(evidence, 0)

		// Query-independent candidate properties (freshness, velocity,
		// authority, tag specificity) scale the evidence as bounded
		// multipliers: they can never flip a score's sign, resurrect a
		// zero-evidence candidate, or push the total outside a predictable
		// range. Under the previous mixed composition, additive authority
		// could dominate the sign of a negative blend, and the freshness
		// multiplier made negative scores *better* as issues aged.
		//
		// Note: the DB query also applies recency decay (90-day half-life)
		// to rank the retrieval window. This app-side freshness weight
		// fine-tunes within the already-retrieved candidates.
		combined := evidence
		combined *= issueanalytics.IssueFreshnessWeight(candidate, now)
		combined *= 1 + scoring.SearchVelocityBoost*issueVelocityScore(candidate)
		combined *= 1 + scoring.AuthorityConsumerWt*issueanalytics.IssueAuthority(candidate)
		combined *= 1 - issueSpecificityPenalty(candidateSummary.Tags, tagSpecificity)
		sharedTags := sharedRelevantTags(querySummary.Tags, candidateSummary.Tags, scoring.SharedTagsLimit)

		scores = append(scores, scoredResult{
			combined:   combined,
			semantic:   semanticSim,
			factor:     factorSim,
			confidence: issueanalytics.ComputeContentConfidence(candidate.Raw),
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

// queryMatchesTagNames returns true if the query exactly contains a known tag
// name as a token-bounded phrase, indicating the user is searching by
// tag-space concepts.
func queryMatchesTagNames(queryLower string, tagNames []string) bool {
	if queryLower == "" || len(tagNames) == 0 {
		return false
	}
	queryTerms := normalizedTagTerms(queryLower)
	for _, tag := range tagNames {
		if containsTagPhrase(queryTerms, normalizedTagTerms(tag)) {
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
		totalPenalty += specificityPenalty(tagSpecificity[domain.NormalizeTagName(tag.Tag)])
	}
	return totalPenalty / float64(limit)
}

// queryMatchesGenericTag returns true if the query exactly contains a tag with
// low specificity (< 0.5), indicating the user is filtering by a broad
// category.
func queryMatchesGenericTag(queryLower string, tagSpecificity map[string]*float64) bool {
	if queryLower == "" {
		return false
	}
	queryTerms := normalizedTagTerms(queryLower)
	for tag, p := range tagSpecificity {
		if !containsTagPhrase(queryTerms, normalizedTagTerms(tag)) {
			continue
		}
		s := scoring.GenericTagThreshold
		if p != nil {
			s = *p
		}
		if s < scoring.GenericTagThreshold {
			return true
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
		if p, ok := tagSpecificity[domain.NormalizeTagName(tag.Tag)]; ok && p != nil {
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
		name := domain.NormalizeTagName(tags[i].Name)
		if name == "" {
			continue
		}
		m[name] = tags[i].Specificity
	}
	return m
}

func normalizedTagTerms(text string) []string {
	return strings.Fields(domain.NormalizeTagName(text))
}

func containsTagPhrase(queryTerms, tagTerms []string) bool {
	if len(queryTerms) == 0 || len(tagTerms) == 0 || len(tagTerms) > len(queryTerms) {
		return false
	}
	for i := 0; i <= len(queryTerms)-len(tagTerms); i++ {
		matched := true
		for j, term := range tagTerms {
			if queryTerms[i+j] != term {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
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
		queryTags = append(queryTags, copyRuntimeTagRelevance(tag, name))
	}

	slices.SortFunc(queryTags, func(a, b TagRelevance) int {
		if diff := cmp.Compare(b.Relevance, a.Relevance); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tag, b.Tag)
	})

	return queryTags
}

// candidateInRegion reports whether the candidate's tag scores include
// the target tag at or above scoring.RegionMembershipFloor.
func candidateInRegion(candidateTags []TagRelevance, target string) bool {
	target = domain.NormalizeTagName(target)
	if target == "" {
		return false
	}
	for _, tag := range candidateTags {
		if domain.NormalizeTagName(tag.Tag) == target &&
			tag.Relevance >= scoring.RegionMembershipFloor {
			return true
		}
	}
	return false
}

// candidateAntiCorrelationPenalty returns the penalty to subtract from
// the combined score when a candidate's strong tags structurally
// dis-occur with the query's target region.
func candidateAntiCorrelationPenalty(
	candidateTags []TagRelevance,
	antiCorrelators map[string]float64,
) float64 {
	penalty := 0.0
	for _, tag := range candidateTags {
		if tag.Relevance < scoring.RegionAntiCorrelationStrongTag {
			continue
		}
		name := domain.NormalizeTagName(tag.Tag)
		weight, ok := antiCorrelators[name]
		if !ok {
			continue
		}
		penalty += weight * scoring.RegionAntiCorrelationPenalty
	}
	return penalty
}
