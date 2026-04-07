package search

import (
	"context"
	"errors"
	"strings"
	"time"

	"splat/internal/ai"
	issueenrichment "splat/internal/issueenrichment"
	"splat/internal/issues"
	issueviews "splat/internal/issues/views"
	issuemap "splat/internal/map"
	"splat/internal/tags"
)

type SearchUnified struct {
	Query string
	Limit int
}

type SearchUnifiedResponse struct {
	Query       issuemap.SearchQuery    `json:"query"`
	Issues      []issuemap.RelatedIssue `json:"issues"`
	RelatedTags []issuemap.RelatedTag   `json:"relatedTags"`
}

type SearchUnifiedHandler struct {
	Analyzer *ai.Analyzer
	Catalog  *tags.CatalogService
	Store    issues.Store
}

func (h SearchUnifiedHandler) Handle(ctx context.Context, input SearchUnified) (SearchUnifiedResponse, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return SearchUnifiedResponse{}, errors.New("query is required")
	}

	limit := input.Limit

	taxonomy, err := h.Catalog.IssueTaxonomy(ctx, nil)
	if err != nil {
		return SearchUnifiedResponse{}, err
	}

	analyzed, err := h.Analyzer.AnalyzeIssueData(ctx, query, taxonomy, nil)
	if err != nil {
		return SearchUnifiedResponse{}, err
	}
	queryEmbedding := issueenrichment.Float32VectorToFloat64(analyzed.Embedding.Vector)
	queryTags := issueenrichment.IssueTagScoresFromAnalysis(analyzed.Tags)

	if searcher, ok := semanticSearchStore(h.Store); ok {
		storeTags, err := h.Catalog.StoredTags(ctx)
		if err != nil {
			return SearchUnifiedResponse{}, err
		}
		candidates, err := semanticSearchCandidateIssues(ctx, searcher, issues.SemanticSearchOptions{
			QueryText:      query,
			QueryEmbedding: queryEmbedding,
			Status:         issues.StatusOpen,
			Limit:          semanticSearchCandidateLimit(limit, 0),
		})
		if err != nil {
			return SearchUnifiedResponse{}, err
		}
		detailReader, _ := h.Store.(issues.IssueDetailReader)
		candidates = issueviews.HydrateIssuesWithVelocity(ctx, detailReader, candidates, time.Now().UTC())

		issueResult := issuemap.SearchFromQueryWithTags(
			candidates,
			storeTags,
			query,
			queryTags,
			queryEmbedding,
			limit,
		)
		relatedTags := issuemap.SearchTags(storeTags, queryEmbedding, limit)

		return SearchUnifiedResponse{
			Query:       issueResult.Query,
			Issues:      issueResult.RelatedIssues,
			RelatedTags: relatedTags,
		}, nil
	}

	storeIssues, err := h.Store.List(ctx)
	if err != nil {
		return SearchUnifiedResponse{}, err
	}
	storeIssues = issueviews.FilterIssuesByStatus(storeIssues, issueviews.IssueStatusFilterOpen)
	detailReader, _ := h.Store.(issues.IssueDetailReader)
	storeIssues = issueviews.HydrateIssuesWithVelocity(ctx, detailReader, storeIssues, time.Now().UTC())

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return SearchUnifiedResponse{}, err
	}

	issueResult := issuemap.SearchFromQueryWithTags(
		storeIssues,
		storeTags,
		query,
		queryTags,
		queryEmbedding,
		limit,
	)

	relatedTags := issuemap.SearchTags(storeTags, queryEmbedding, limit)

	return SearchUnifiedResponse{
		Query:       issueResult.Query,
		Issues:      issueResult.RelatedIssues,
		RelatedTags: relatedTags,
	}, nil
}
