package search

import (
	"context"
	"errors"
	"strings"
	"time"

	"sortit/internal/ai"
	"sortit/internal/centering"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
	issueviews "sortit/internal/issues/views"
	issuemap "sortit/internal/map"
	"sortit/internal/ridgedecomp"
	"sortit/internal/ridgelambda"
	"sortit/internal/tags"
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
	// Centering provides revision-cached full-corpus embedding means —
	// see SearchIssuesHandler.Centering.
	Centering *centering.Cache
	// RidgeDecomp provides the revision-cached full-corpus ridge decomposition,
	// preferred over RidgeLambda — see SearchIssuesHandler.RidgeDecomp.
	RidgeDecomp *ridgedecomp.Cache
	// RidgeLambda selects the anchored-ridge similarity model when wired —
	// see SearchIssuesHandler.RidgeLambda.
	RidgeLambda *ridgelambda.Cache
}

func (h SearchUnifiedHandler) ridgeOption(ctx context.Context) ([]issuemap.SearchOption, error) {
	if h.RidgeDecomp != nil {
		decomp, ok, err := h.RidgeDecomp.Current(ctx)
		if err != nil {
			return nil, err
		}
		if ok {
			return []issuemap.SearchOption{issuemap.WithRidgeDecomposition(*decomp)}, nil
		}
	}
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

	analyzed, err := h.Analyzer.AnalyzeIssueData(ctx, query, taxonomy, nil, ai.ConceptFrame{})
	if err != nil {
		return SearchUnifiedResponse{}, err
	}
	queryEmbedding := issueenrichment.Float32VectorToFloat64(analyzed.Embedding.Vector)
	queryTags := issueenrichment.IssueTagScoresFromAnalysis(analyzed.Tags)

	corpusMeans, err := h.Centering.Current(ctx)
	if err != nil {
		return SearchUnifiedResponse{}, err
	}

	ridgeOpt, err := h.ridgeOption(ctx)
	if err != nil {
		return SearchUnifiedResponse{}, err
	}

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
			append([]issuemap.SearchOption{issuemap.WithCorpusMeans(corpusMeans)}, ridgeOpt...)...,
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
		append([]issuemap.SearchOption{issuemap.WithCorpusMeans(corpusMeans)}, ridgeOpt...)...,
	)

	relatedTags := issuemap.SearchTags(storeTags, queryEmbedding, limit)

	return SearchUnifiedResponse{
		Query:       issueResult.Query,
		Issues:      issueResult.RelatedIssues,
		RelatedTags: relatedTags,
	}, nil
}
