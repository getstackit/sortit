package queries

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"

	"splat/internal/ai"
	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
)

type IssueStatusFilter string

const (
	IssueStatusFilterOpen   IssueStatusFilter = "open"
	IssueStatusFilterClosed IssueStatusFilter = "closed"
	IssueStatusFilterAll    IssueStatusFilter = "all"
)

type ListIssuesHandler struct {
	Store issues.Store
}

func (h ListIssuesHandler) Handle(ctx context.Context, filter IssueStatusFilter) ([]issues.Issue, error) {
	items, err := h.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	return FilterIssuesByStatus(items, filter), nil
}

type GetIssueHandler struct {
	Store issues.Store
}

func (h GetIssueHandler) Handle(ctx context.Context, id string) (issues.Issue, error) {
	return h.Store.Get(ctx, id)
}

type CompareIssues struct {
	IDs []string
}

type PairwiseIssueSimilarity struct {
	SourceID   string
	TargetID   string
	Similarity float64
}

type CompareIssuesResult struct {
	ComparedIssueCount          int
	AverageEmbeddingSimilarity  float64
	PairwiseEmbeddingSimilarity []PairwiseIssueSimilarity
}

type CompareIssuesHandler struct {
	Store issues.Store
}

func (h CompareIssuesHandler) Handle(ctx context.Context, input CompareIssues) (CompareIssuesResult, error) {
	storeIssues, err := h.Store.List(ctx)
	if err != nil {
		return CompareIssuesResult{}, err
	}

	issuesByID := make(map[string]issues.Issue, len(storeIssues))
	for _, issue := range storeIssues {
		issuesByID[issue.ID] = issue
	}

	selected := make([]issues.Issue, 0, len(input.IDs))
	for _, id := range input.IDs {
		issue, ok := issuesByID[id]
		if !ok {
			return CompareIssuesResult{}, issues.ErrNotFound
		}
		selected = append(selected, issue)
	}

	pairs, average := compareIssueEmbeddings(selected)
	return CompareIssuesResult{
		ComparedIssueCount:          len(selected),
		AverageEmbeddingSimilarity:  average,
		PairwiseEmbeddingSimilarity: pairs,
	}, nil
}

type SearchIssues struct {
	Query  string
	Limit  int
	Status IssueStatusFilter
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

	storeIssues, err := h.Store.List(ctx)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}
	storeIssues = FilterIssuesByStatus(storeIssues, input.Status)

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return issuemap.SearchResponse{}, err
	}

	return issuemap.SearchFromQueryWithTags(
		storeIssues,
		storeTags,
		query,
		services.IssueTagScoresFromAnalysis(analyzed.Tags),
		services.Float32VectorToFloat64(analyzed.Embedding.Vector),
		input.Limit,
	), nil
}

func FilterIssuesByStatus(items []issues.Issue, filter IssueStatusFilter) []issues.Issue {
	if filter == IssueStatusFilterAll {
		return items
	}

	filtered := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		status := item.Status
		if status == "" {
			status = issues.StatusOpen
		}

		if filter == IssueStatusFilterOpen && status == issues.StatusOpen {
			filtered = append(filtered, item)
		}
		if filter == IssueStatusFilterClosed && status == issues.StatusClosed {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func compareIssueEmbeddings(items []issues.Issue) ([]PairwiseIssueSimilarity, float64) {
	pairs := make([]PairwiseIssueSimilarity, 0, len(items)*(len(items)-1)/2)
	total := 0.0

	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			similarity := CosineSimilarity(items[i].Embedding, items[j].Embedding)
			total += similarity
			pairs = append(pairs, PairwiseIssueSimilarity{
				SourceID:   items[i].ID,
				TargetID:   items[j].ID,
				Similarity: math.Round(similarity*100) / 100,
			})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Similarity == pairs[j].Similarity {
			if pairs[i].SourceID == pairs[j].SourceID {
				return pairs[i].TargetID < pairs[j].TargetID
			}
			return pairs[i].SourceID < pairs[j].SourceID
		}
		return pairs[i].Similarity > pairs[j].Similarity
	})

	average := 0.0
	if len(pairs) > 0 {
		average = math.Round((total/float64(len(pairs)))*100) / 100
	}
	return pairs, average
}

func NotFound(err error) bool {
	return errors.Is(err, issues.ErrNotFound)
}
