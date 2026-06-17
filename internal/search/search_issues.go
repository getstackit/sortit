package search

import (
	"context"
	"errors"
	"strings"
	"time"

	"sortit/internal/ai"
	"sortit/internal/centering"
	"sortit/internal/domain"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
	issueviews "sortit/internal/issues/views"
	issuemap "sortit/internal/map"
	"sortit/internal/ridgelambda"
	"sortit/internal/tagcooccurrence"
	"sortit/internal/tags"
)

// regionSearchOptsTopK caps the number of anti-correlators the search
// passes to the ranker. Going wider buys diminishing returns and risks
// over-penalizing candidates that touch many unrelated tags.
const regionSearchOptsTopK = 8

type SearchIssues struct {
	Query      string
	Limit      int
	Offset     int
	Status     issueviews.IssueStatusFilter
	AssignedTo string
	Tags       []string
	SortBy     string // "relevance" (default), "created_at", "updated_at"
}

type SearchIssuesHandler struct {
	Analyzer     *ai.Analyzer
	Catalog      *tags.CatalogService
	Store        issues.Store
	Cooccurrence *tagcooccurrence.Cache
	// Centering provides revision-cached full-corpus embedding means.
	// Critical on the semantic-search path: candidates are a retrieved
	// subset, and centering them with their own mean would subtract the
	// query signal itself.
	Centering *centering.Cache
	// RidgeLambda provides the revision-cached GCV penalty that selects the
	// anchored-ridge similarity model. When wired and the corpus is large
	// enough, ridge is the default ranker; otherwise search falls back to
	// the rank-1 factor model.
	RidgeLambda *ridgelambda.Cache
}

// ridgeOption returns the WithRidgeSimilarity option carrying the
// revision-cached GCV penalty, or an empty slice when ridge is unavailable
// (cache not wired, or corpus too small) so search uses the rank-1 model.
func (h SearchIssuesHandler) ridgeOption(ctx context.Context) ([]issuemap.SearchOption, error) {
	if h.RidgeLambda == nil {
		return nil, nil
	}
	lambda, ok, err := h.RidgeLambda.Current(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return []issuemap.SearchOption{issuemap.WithRidgeSimilarity(lambda)}, nil
}

// regionOpts inspects the query for an exact tag-name match. When found,
// it returns options that boost in-region candidates and penalize
// candidates carrying that region's strong anti-correlators. Returns
// an empty slice when no tag matches or when the cooccurrence cache is
// not wired.
func (h SearchIssuesHandler) regionOpts(
	ctx context.Context,
	query string,
	storeTags []issues.Tag,
) ([]issuemap.SearchOption, error) {
	normalized := domain.NormalizeTagName(query)
	if normalized == "" {
		return nil, nil
	}
	var match string
	for _, tag := range storeTags {
		if domain.NormalizeTagName(tag.Name) == normalized {
			match = normalized
			break
		}
	}
	if match == "" {
		return nil, nil
	}

	opts := []issuemap.SearchOption{issuemap.WithRegionTarget(match)}
	if h.Cooccurrence == nil {
		return opts, nil
	}
	stats, ok, err := h.Cooccurrence.LookupPairs(ctx, match)
	if err != nil {
		return nil, err
	}
	if !ok {
		return opts, nil
	}
	anti := make(map[string]float64, regionSearchOptsTopK)
	for _, pair := range stats.Pairs {
		if pair.ImplicitNegative <= 0 {
			continue
		}
		if len(anti) >= regionSearchOptsTopK {
			break
		}
		anti[pair.Tag] = pair.ImplicitNegative
	}
	if len(anti) > 0 {
		opts = append(opts, issuemap.WithAntiCorrelators(anti))
	}
	return opts, nil
}

func (h SearchIssuesHandler) Handle(ctx context.Context, input SearchIssues) (issuemap.SearchResponse, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return issuemap.SearchResponse{}, errors.New("query is required")
	}
	if input.Status == "" {
		input.Status = issueviews.IssueStatusFilterOpen
	}

	taxonomy, err := h.Catalog.IssueTaxonomy(ctx, nil)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}

	analyzed, err := h.Analyzer.AnalyzeIssueData(ctx, query, taxonomy, nil)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}

	searchOpts := issuemap.SearchOptions{
		Query:      query,
		QueryTags:  issueenrichment.IssueTagScoresFromAnalysis(analyzed.Tags),
		QueryEmbed: issueenrichment.Float32VectorToFloat64(analyzed.Embedding.Vector),
		Limit:      input.Limit,
		Offset:     input.Offset,
		SortBy:     input.SortBy,
	}

	corpusMeans, err := h.Centering.Current(ctx)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}

	ridgeOpt, err := h.ridgeOption(ctx)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}

	if searcher, ok := semanticSearchStore(h.Store); ok {
		storeTags, err := h.Catalog.StoredTags(ctx)
		if err != nil {
			return issuemap.SearchResponse{}, err
		}
		candidates, err := semanticSearchCandidateIssues(ctx, searcher, issues.SemanticSearchOptions{
			QueryText:      searchOpts.Query,
			QueryEmbedding: searchOpts.QueryEmbed,
			Status:         issueviews.IssueStatusFromFilter(input.Status),
			AssignedTo:     input.AssignedTo,
			Tags:           input.Tags,
			Limit:          semanticSearchCandidateLimit(searchOpts.Limit, searchOpts.Offset),
			SortBy:         searchOpts.SortBy,
		})
		if err != nil {
			return issuemap.SearchResponse{}, err
		}
		detailReader, _ := h.Store.(issues.IssueDetailReader)
		candidates = issueviews.HydrateIssuesWithVelocity(ctx, detailReader, candidates, time.Now().UTC())

		regionExtras, err := h.regionOpts(ctx, searchOpts.Query, storeTags)
		if err != nil {
			return issuemap.SearchResponse{}, err
		}

		baseOpts := []issuemap.SearchOption{
			issuemap.WithOffset(searchOpts.Offset),
			issuemap.WithSortBy(searchOpts.SortBy),
			issuemap.WithCorpusMeans(corpusMeans),
		}
		baseOpts = append(baseOpts, ridgeOpt...)

		return issuemap.SearchFromQueryWithTags(
			candidates,
			storeTags,
			searchOpts.Query,
			searchOpts.QueryTags,
			searchOpts.QueryEmbed,
			searchOpts.Limit,
			append(baseOpts, regionExtras...)...,
		), nil
	}

	storeIssues, err := h.Store.List(ctx)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}
	storeIssues = issueviews.FilterIssuesByStatus(storeIssues, input.Status)
	storeIssues = issueviews.FilterIssuesByAssignee(storeIssues, input.AssignedTo)
	storeIssues = issueviews.FilterIssuesByTags(storeIssues, input.Tags)
	detailReader, _ := h.Store.(issues.IssueDetailReader)
	storeIssues = issueviews.HydrateIssuesWithVelocity(ctx, detailReader, storeIssues, time.Now().UTC())

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}

	regionExtras, err := h.regionOpts(ctx, searchOpts.Query, storeTags)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}

	baseOpts := []issuemap.SearchOption{
		issuemap.WithOffset(searchOpts.Offset),
		issuemap.WithSortBy(searchOpts.SortBy),
		issuemap.WithCorpusMeans(corpusMeans),
	}
	baseOpts = append(baseOpts, ridgeOpt...)

	return issuemap.SearchFromQueryWithTags(
		storeIssues,
		storeTags,
		searchOpts.Query,
		searchOpts.QueryTags,
		searchOpts.QueryEmbed,
		searchOpts.Limit,
		append(baseOpts, regionExtras...)...,
	), nil
}
