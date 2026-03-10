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
	Catalog  *services.CatalogService
	Store    issues.Store
	Corpus   *DerivedCorpusLoader
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

	analyzed, err := h.Analyzer.AnalyzeIssueData(ctx, query, taxonomy)
	if err != nil {
		return SearchUnifiedResponse{}, err
	}

	if h.Corpus != nil {
		corpus, err := h.Corpus.Current(ctx)
		if err != nil {
			return SearchUnifiedResponse{}, err
		}
		filtered := FilterIssuesByStatus(corpus.Issues, IssueStatusFilterOpen)
		corpus = subsetCorpusByIssues(corpus, filtered)
		queryEmbedding := services.Float32VectorToFloat64(analyzed.Embedding.Vector)
		issueResult := issuemap.SearchFromCorpus(
			corpus,
			query,
			services.IssueTagScoresFromAnalysis(analyzed.Tags),
			queryEmbedding,
			limit,
		)

		relatedTags := issuemap.SearchTags(corpus.Tags, queryEmbedding, limit)

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
	storeIssues = FilterIssuesByStatus(storeIssues, IssueStatusFilterOpen)

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return SearchUnifiedResponse{}, err
	}

	queryEmbedding := services.Float32VectorToFloat64(analyzed.Embedding.Vector)

	issueResult := issuemap.SearchFromQueryWithTags(
		storeIssues,
		storeTags,
		query,
		services.IssueTagScoresFromAnalysis(analyzed.Tags),
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
