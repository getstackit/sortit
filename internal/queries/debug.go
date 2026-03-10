package queries

import (
	"context"
	"errors"
	"math"
	"sort"

	"splat/internal/ai"
	"splat/internal/issues"
	"splat/internal/services"
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
	Analyzer  *ai.Analyzer
	Catalog   *services.CatalogService
	Store     issues.Store
	ReadModel *ReadModelLoader
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
	var storeIssues []issues.Issue
	if h.Store != nil {
		items, err := h.Store.List(ctx)
		if err != nil {
			return nil, 0, 0, err
		}
		storeIssues = items
	} else if h.ReadModel != nil {
		model, err := h.ReadModel.Current(ctx)
		if err != nil {
			return nil, 0, 0, err
		}
		storeIssues = model.Corpus.Issues
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

		similarity := CosineSimilarity(query, issue.Embedding)
		compared++
		total += similarity
		comparisons = append(comparisons, DebugIssueSimilarity{
			ID:         issue.ID,
			Raw:        issue.Raw,
			Tags:       append([]string(nil), issue.Tags...),
			Similarity: math.Round(similarity*100) / 100,
		})
	}

	sort.Slice(comparisons, func(i, j int) bool {
		if comparisons[i].Similarity == comparisons[j].Similarity {
			return comparisons[i].ID < comparisons[j].ID
		}
		return comparisons[i].Similarity > comparisons[j].Similarity
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
