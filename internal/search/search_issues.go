package search

import (
	"context"
	"errors"
	"strings"
	"time"

	"sortit/internal/ai"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
	issueviews "sortit/internal/issues/views"
	issuemap "sortit/internal/map"
	"sortit/internal/tags"
)

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
	Analyzer *ai.Analyzer
	Catalog  *tags.CatalogService
	Store    issues.Store
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
	storeIssues = issueviews.FilterIssuesByStatus(storeIssues, input.Status)
	storeIssues = issueviews.FilterIssuesByAssignee(storeIssues, input.AssignedTo)
	storeIssues = issueviews.FilterIssuesByTags(storeIssues, input.Tags)
	detailReader, _ := h.Store.(issues.IssueDetailReader)
	storeIssues = issueviews.HydrateIssuesWithVelocity(ctx, detailReader, storeIssues, time.Now().UTC())

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
