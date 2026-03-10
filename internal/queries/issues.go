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
	Store     issues.Store
	ReadModel *ReadModelLoader
}

func (h CompareIssuesHandler) Handle(ctx context.Context, input CompareIssues) (CompareIssuesResult, error) {
	if h.ReadModel != nil {
		model, err := h.ReadModel.Current(ctx)
		if err != nil {
			return CompareIssuesResult{}, err
		}

		selected := make([]issues.Issue, 0, len(input.IDs))
		for _, id := range input.IDs {
			issue, ok := model.Corpus.IssuesByID[id]
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
	Query      string
	Limit      int
	Offset     int
	Status     IssueStatusFilter
	AssignedTo string
	Tags       []string
	SortBy     string // "relevance" (default), "created_at", "updated_at"
}

type SearchIssuesHandler struct {
	Analyzer  *ai.Analyzer
	Catalog   *services.CatalogService
	Store     issues.Store
	ReadModel *ReadModelLoader
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

	if h.ReadModel != nil {
		model, err := h.ReadModel.Current(ctx)
		if err != nil {
			return issuemap.SearchResponse{}, err
		}
		filtered := FilterIssuesByStatus(model.Issues, input.Status)
		filtered = filterIssuesByAssignee(filtered, input.AssignedTo)
		filtered = filterIssuesByTags(filtered, input.Tags)
		corpus, err := issuemap.BuildDerivedCorpus(filtered, model.Tags)
		if err != nil {
			return issuemap.SearchResponse{}, err
		}
		return issuemap.SearchFromCorpus(
			corpus,
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

type SearchUnified struct {
	Query string
	Limit int
}

type SearchUnifiedResponse struct {
	Query       issuemap.SearchQuery    `json:"query"`
	Issues      []issuemap.RelatedIssue `json:"issues"`
	RelatedTags []issuemap.RelatedTag   `json:"relatedTags"`
}

type ExploreIssue struct {
	ID    string
	Limit int
}

type ExploreIssueHandler struct {
	Store     issues.Store
	Catalog   *services.CatalogService
	ReadModel *ReadModelLoader
}

func (h ExploreIssueHandler) Handle(ctx context.Context, input ExploreIssue) (issuemap.ExploreResponse, error) {
	if h.ReadModel != nil {
		model, err := h.ReadModel.Current(ctx)
		if err != nil {
			return issuemap.ExploreResponse{}, err
		}
		return issuemap.ExploreFromCorpus(model.Corpus, input.ID, input.Limit)
	}
	storeIssues, err := h.Store.List(ctx)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}

	return issuemap.ExploreFromIssuesWithTags(storeIssues, storeTags, input.ID, input.Limit)
}

type SearchUnifiedHandler struct {
	Analyzer  *ai.Analyzer
	Catalog   *services.CatalogService
	Store     issues.Store
	ReadModel *ReadModelLoader
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

	if h.ReadModel != nil {
		model, err := h.ReadModel.Current(ctx)
		if err != nil {
			return SearchUnifiedResponse{}, err
		}
		filtered := FilterIssuesByStatus(model.Issues, IssueStatusFilterOpen)
		corpus, err := issuemap.BuildDerivedCorpus(filtered, model.Tags)
		if err != nil {
			return SearchUnifiedResponse{}, err
		}
		queryEmbedding := services.Float32VectorToFloat64(analyzed.Embedding.Vector)
		issueResult := issuemap.SearchFromCorpus(
			corpus,
			query,
			services.IssueTagScoresFromAnalysis(analyzed.Tags),
			queryEmbedding,
			limit,
		)

		relatedTags := issuemap.SearchTags(model.Tags, queryEmbedding, limit)

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

	for i := range items {
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

func filterIssuesByAssignee(items []issues.Issue, assignedTo string) []issues.Issue {
	assignedTo = strings.TrimSpace(assignedTo)
	if assignedTo == "" {
		return items
	}
	filtered := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(item.AssignedTo, assignedTo) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterIssuesByTags(items []issues.Issue, tags []string) []issues.Issue {
	if len(tags) == 0 {
		return items
	}
	required := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag != "" {
			required[tag] = struct{}{}
		}
	}
	if len(required) == 0 {
		return items
	}
	filtered := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		if issueMatchesTags(item, required) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func issueMatchesTags(item issues.Issue, required map[string]struct{}) bool {
	for _, tag := range item.Tags {
		if _, ok := required[strings.ToLower(tag)]; ok {
			return true
		}
	}
	for _, ts := range item.TagScores {
		if ts.Relevance < 0.3 {
			continue
		}
		if _, ok := required[strings.ToLower(ts.Tag)]; ok {
			return true
		}
	}
	return false
}

func NotFound(err error) bool {
	return errors.Is(err, issues.ErrNotFound)
}
