package services

import (
	"context"

	"splat/internal/ai"
	"splat/internal/issues"
)

type IssueEnricher struct {
	analyzer *ai.Analyzer
	catalog  *CatalogService
}

func NewIssueEnricher(analyzer *ai.Analyzer, catalog *CatalogService) *IssueEnricher {
	return &IssueEnricher{
		analyzer: analyzer,
		catalog:  catalog,
	}
}

func (s *IssueEnricher) AnalyzeCreateInput(ctx context.Context, input issues.CreateInput) (issues.CreateInput, error) {
	taxonomy, err := s.catalog.IssueTaxonomy(ctx, input.Tags)
	if err != nil {
		return issues.CreateInput{}, err
	}
	analyzed, err := s.analyzer.AnalyzeIssueData(ctx, input.Raw, taxonomy)
	if err != nil {
		return issues.CreateInput{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, CatalogTagsFromAnalysis(taxonomy, input.Tags, analyzed.Tags)); err != nil {
		return issues.CreateInput{}, err
	}

	input.TagScores = IssueTagScoresFromAnalysis(analyzed.Tags)
	input.Embedding = Float32VectorToFloat64(analyzed.Embedding.Vector)
	if len(input.Tags) == 0 {
		input.Tags = nil
	}
	return input, nil
}

func (s *IssueEnricher) AnalyzeSeedIssues(ctx context.Context, seeds []issues.Issue) ([]issues.Issue, error) {
	enriched := make([]issues.Issue, len(seeds))
	for i, seed := range seeds {
		analyzed, err := s.AnalyzeCreateInput(ctx, issues.CreateInput{
			Raw:  seed.Raw,
			Tags: seed.Tags,
		})
		if err != nil {
			return nil, err
		}

		enriched[i] = issues.Issue{
			ID:        seed.ID,
			Raw:       seed.Raw,
			Tags:      append([]string(nil), seed.Tags...),
			CreatedBy: seed.CreatedBy,
			CreatedAt: seed.CreatedAt,
			TagScores: analyzed.TagScores,
			Embedding: analyzed.Embedding,
			Status:    seed.Status,
			ClosedAt:  seed.ClosedAt,
			ClosedBy:  seed.ClosedBy,
		}
	}
	return enriched, nil
}

func IssueTagScoresFromAnalysis(scores []ai.TagScore) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}

	tagScores := make([]issues.TagRelevance, 0, len(scores))
	for _, score := range scores {
		name := score.Tag
		if name == "" {
			continue
		}
		tagScores = append(tagScores, issues.TagRelevance{
			Tag:       name,
			Relevance: score.Relevance,
		})
	}
	return tagScores
}

func Float32VectorToFloat64(values []float32) []float64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]float64, len(values))
	for i, value := range values {
		out[i] = float64(value)
	}
	return out
}
