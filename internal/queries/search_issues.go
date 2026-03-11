package queries

import (
	"context"
	"errors"
	"strings"

	"splat/internal/ai"
	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
)

type SearchIssues struct {
	Query      string
	Limit      int
	Offset     int
	Status     IssueStatusFilter
	AssignedTo string
	Tags       []string
	SortBy     string // "relevance" (default), "created_at", "updated_at"
}

type SearchIssuesHandler struct {
	Analyzer *ai.Analyzer
	Catalog  *services.CatalogService
	Store    issues.Store
}

func (h SearchIssuesHandler) Handle(ctx context.Context, input SearchIssues) (issuemap.SearchResponse, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return issuemap.SearchResponse{}, errors.New("query is required")
	}
	if input.Status == "" {
		input.Status = IssueStatusFilterOpen
	}

	taxonomy, err := h.Catalog.IssueTaxonomy(ctx, nil)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}

	analyzed, err := h.Analyzer.AnalyzeIssueData(ctx, query, taxonomy)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}

	searchOpts := issuemap.SearchOptions{
		Query:      query,
		QueryTags:  services.IssueTagScoresFromAnalysis(analyzed.Tags),
		QueryEmbed: services.Float32VectorToFloat64(analyzed.Embedding.Vector),
		Limit:      input.Limit,
		Offset:     input.Offset,
		SortBy:     input.SortBy,
	}

	if searcher, ok := semanticSearchStore(h.Store); ok {
		storeTags, err := h.Catalog.StoredTags(ctx)
		if err != nil {
			return issuemap.SearchResponse{}, err
		}
		candidates, err := semanticSearchCandidateIssues(ctx, searcher, issues.SemanticSearchOptions{
			QueryEmbedding: searchOpts.QueryEmbed,
			Status:         issueStatusFromFilter(input.Status),
			AssignedTo:     input.AssignedTo,
			Tags:           input.Tags,
			Limit:          semanticSearchCandidateLimit(searchOpts.Limit, searchOpts.Offset),
			SortBy:         searchOpts.SortBy,
		})
		if err != nil {
			return issuemap.SearchResponse{}, err
		}

		return issuemap.SearchFromQueryWithTags(
			candidates,
			storeTags,
			searchOpts.Query,
			searchOpts.QueryTags,
			searchOpts.QueryEmbed,
			searchOpts.Limit,
			issuemap.WithOffset(searchOpts.Offset),
			issuemap.WithSortBy(searchOpts.SortBy),
		), nil
	}

	storeIssues, err := h.Store.List(ctx)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}
	storeIssues = FilterIssuesByStatus(storeIssues, input.Status)
	storeIssues = filterIssuesByAssignee(storeIssues, input.AssignedTo)
	storeIssues = filterIssuesByTags(storeIssues, input.Tags)

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}

	return issuemap.SearchFromQueryWithTags(
		storeIssues,
		storeTags,
		searchOpts.Query,
		searchOpts.QueryTags,
		searchOpts.QueryEmbed,
		searchOpts.Limit,
		issuemap.WithOffset(searchOpts.Offset),
		issuemap.WithSortBy(searchOpts.SortBy),
	), nil
}
