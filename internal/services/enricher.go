package services

import (
	"context"
	"fmt"
	"strings"

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

func (s *IssueEnricher) AnalyzeRefineInput(ctx context.Context, issue issues.Issue, postRaw string, createdBy string) (issues.RefineInput, error) {
	postRaw = strings.TrimSpace(postRaw)
	if postRaw == "" {
		return issues.RefineInput{}, fmt.Errorf("post raw is required")
	}

	discussionTexts := make([]string, 0, len(issue.Discussion)+1)
	for _, post := range issue.Discussion {
		text := strings.TrimSpace(post.Raw)
		if text == "" {
			continue
		}
		discussionTexts = append(discussionTexts, text)
	}
	discussionTexts = append(discussionTexts, postRaw)

	canonicalRaw, err := s.analyzer.CanonicalizeDiscussion(ctx, discussionTexts)
	if err != nil {
		return issues.RefineInput{}, err
	}
	canonicalRaw = strings.TrimSpace(canonicalRaw)
	if canonicalRaw == "" {
		return issues.RefineInput{}, fmt.Errorf("canonical raw is required")
	}

	taxonomy, err := s.catalog.IssueTaxonomy(ctx, nil)
	if err != nil {
		return issues.RefineInput{}, err
	}
	analyzed, err := s.analyzer.AnalyzeIssueData(ctx, canonicalRaw, taxonomy)
	if err != nil {
		return issues.RefineInput{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, CatalogTagsFromAnalysis(taxonomy, nil, analyzed.Tags)); err != nil {
		return issues.RefineInput{}, err
	}

	return issues.RefineInput{
		PostRaw:      postRaw,
		CanonicalRaw: canonicalRaw,
		CreatedBy:    createdBy,
		TagScores:    IssueTagScoresFromAnalysis(analyzed.Tags),
		Embedding:    Float32VectorToFloat64(analyzed.Embedding.Vector),
	}, nil
}

func (s *IssueEnricher) AnalyzeCombineInput(
	ctx context.Context,
	sourceIssues []issues.Issue,
	createdBy string,
	note string,
) (issues.CombineInput, error) {
	if len(sourceIssues) < 2 {
		return issues.CombineInput{}, fmt.Errorf("at least two source issues are required")
	}

	parts := make([]string, 0, len(sourceIssues)+1)
	sourceIDs := make([]string, 0, len(sourceIssues))
	for _, issue := range sourceIssues {
		raw := strings.TrimSpace(issue.Raw)
		if raw == "" {
			continue
		}
		parts = append(parts, raw)
		sourceIDs = append(sourceIDs, issue.ID)
	}
	if extra := strings.TrimSpace(note); extra != "" {
		parts = append(parts, "Combination rationale: "+extra)
	}

	canonicalRaw, err := s.analyzer.CanonicalizeDiscussion(ctx, parts)
	if err != nil {
		return issues.CombineInput{}, err
	}
	canonicalRaw = strings.TrimSpace(canonicalRaw)
	if canonicalRaw == "" {
		return issues.CombineInput{}, fmt.Errorf("canonical raw is required")
	}

	taxonomy, err := s.catalog.IssueTaxonomy(ctx, nil)
	if err != nil {
		return issues.CombineInput{}, err
	}
	analyzed, err := s.analyzer.AnalyzeIssueData(ctx, canonicalRaw, taxonomy)
	if err != nil {
		return issues.CombineInput{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, CatalogTagsFromAnalysis(taxonomy, nil, analyzed.Tags)); err != nil {
		return issues.CombineInput{}, err
	}

	return issues.CombineInput{
		SourceIDs: sourceIDs,
		Raw:       canonicalRaw,
		CreatedBy: createdBy,
		Note:      note,
		TagScores: IssueTagScoresFromAnalysis(analyzed.Tags),
		Embedding: Float32VectorToFloat64(analyzed.Embedding.Vector),
	}, nil
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
