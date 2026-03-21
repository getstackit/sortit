package queries

import (
	"cmp"
	"context"
	"errors"
	"math"
	"slices"

	"splat/internal/ai"
	"splat/internal/issues"
	issuemap "splat/internal/map"
	"splat/internal/services"
	"splat/internal/vectors"
)

type DebugAnalyzeIssue struct {
	Text string
	Tags []string
}

type DebugIssueSimilarity struct {
	ID         string
	Raw        string
	Tags       []string
	Similarity float64
}

type DebugAnalyzeIssueResult struct {
	Tags                   []ai.TagScore
	Embedding              ai.EmbeddingInfo
	Tagger                 ai.ModelInfo
	Embedder               ai.ModelInfo
	ComparedIssueCount     int
	AverageIssueSimilarity float64
	SimilarIssues          []DebugIssueSimilarity
}

type DebugAnalyzeIssueHandler struct {
	Analyzer *ai.Analyzer
	Catalog  *services.CatalogService
	Store    issues.Store
}

type issueEmbeddingSimilarityLister interface {
	ListIssueEmbeddingSimilarities(context.Context, []float64, int) ([]issues.IssueEmbeddingSimilarity, int, float64, error)
}

func (h DebugAnalyzeIssueHandler) Handle(ctx context.Context, input DebugAnalyzeIssue) (DebugAnalyzeIssueResult, error) {
	tags, err := h.Catalog.IssueTaxonomy(ctx, input.Tags)
	if err != nil {
		return DebugAnalyzeIssueResult{}, err
	}

	analyzed, err := h.Analyzer.AnalyzeIssueData(ctx, input.Text, tags)
	if err != nil {
		return DebugAnalyzeIssueResult{}, err
	}

	similarities, comparedIssueCount, averageSimilarity, err := h.issueEmbeddingSimilarities(ctx, services.Float32VectorToFloat64(analyzed.Embedding.Vector))
	if err != nil {
		return DebugAnalyzeIssueResult{}, err
	}

	return DebugAnalyzeIssueResult{
		Tags:                   analyzed.Tags,
		Embedding:              analyzed.Embedding.Info,
		Tagger:                 analyzed.Tagger,
		Embedder:               analyzed.Embedder,
		ComparedIssueCount:     comparedIssueCount,
		AverageIssueSimilarity: averageSimilarity,
		SimilarIssues:          similarities,
	}, nil
}

func (h DebugAnalyzeIssueHandler) issueEmbeddingSimilarities(ctx context.Context, query []float64) ([]DebugIssueSimilarity, int, float64, error) {
	if similarityStore, ok := h.Store.(issueEmbeddingSimilarityLister); ok {
		items, compared, average, err := similarityStore.ListIssueEmbeddingSimilarities(ctx, query, 8)
		if err != nil {
			return nil, 0, 0, err
		}
		if items != nil {
			similarities := make([]DebugIssueSimilarity, 0, len(items))
			for _, item := range items {
				similarities = append(similarities, DebugIssueSimilarity{
					ID:         item.ID,
					Raw:        item.Raw,
					Tags:       append([]string(nil), item.Tags...),
					Similarity: item.Similarity,
				})
			}
			return similarities, compared, average, nil
		}
	}

	var storeIssues []issues.Issue
	if h.Store != nil {
		items, err := h.Store.List(ctx)
		if err != nil {
			return nil, 0, 0, err
		}
		storeIssues = items
	}

	if len(query) == 0 {
		return []DebugIssueSimilarity{}, 0, 0, nil
	}

	comparisons := make([]DebugIssueSimilarity, 0, len(storeIssues))
	total := 0.0
	compared := 0
	for _, issue := range storeIssues {
		if len(issue.Embedding) == 0 {
			continue
		}

		similarity := vectors.CosineSimilarity(query, issue.Embedding)
		compared++
		total += similarity
		comparisons = append(comparisons, DebugIssueSimilarity{
			ID:         issue.ID,
			Raw:        issue.Raw,
			Tags:       append([]string(nil), issue.Tags...),
			Similarity: math.Round(similarity*100) / 100,
		})
	}

	slices.SortStableFunc(comparisons, func(a, b DebugIssueSimilarity) int {
		if c := cmp.Compare(b.Similarity, a.Similarity); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	average := 0.0
	if compared > 0 {
		average = math.Round((total/float64(compared))*100) / 100
	}
	if len(comparisons) > 8 {
		comparisons = comparisons[:8]
	}

	return comparisons, compared, average, nil
}

func AIUnavailable(err error) bool {
	return errors.Is(err, ai.ErrNotConfigured)
}

// DebugFactorWeightsResult holds the current factor decomposition weights
// computed from the issue corpus.
type DebugFactorWeightsResult struct {
	FactorWeight   float64 `json:"factorWeight"`
	ResidualWeight float64 `json:"residualWeight"`
	IssueCount     int     `json:"issueCount"`
	Decomposed     bool    `json:"decomposed"`
}

type DebugFactorWeightsHandler struct {
	Store   issues.Store
	Catalog *services.CatalogService
}

func (h DebugFactorWeightsHandler) Handle(ctx context.Context) (DebugFactorWeightsResult, error) {
	var storeIssues []issues.Issue
	if h.Store != nil {
		items, err := h.Store.List(ctx)
		if err != nil {
			return DebugFactorWeightsResult{}, err
		}
		storeIssues = items
	}

	var storeTags []issues.Tag
	if h.Catalog != nil {
		tags, err := h.Catalog.StoredTags(ctx)
		if err == nil {
			storeTags = tags
		}
	}

	tagNames, tagEmbeddings := personDetailTagData(storeIssues, storeTags)
	issueEmbeddings := make(map[string][]float64, len(storeIssues))
	for _, issue := range storeIssues {
		if len(issue.Embedding) > 0 {
			issueEmbeddings[issue.ID] = issue.Embedding
		}
	}

	decomp := issuemap.ComputeFactorDecomposition(storeIssues, tagNames, issueEmbeddings, tagEmbeddings)

	return DebugFactorWeightsResult{
		FactorWeight:   math.Round(decomp.FactorWeight*1000) / 1000,
		ResidualWeight: math.Round(decomp.ResidualWeight*1000) / 1000,
		IssueCount:     len(storeIssues),
		Decomposed:     len(decomp.FactorEmbeddings) > 0,
	}, nil
}
