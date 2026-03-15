package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"splat/internal/ai"
	"splat/internal/issues"
)

type IssueEnricher struct {
	analyzer *ai.Analyzer
	catalog  *CatalogService
	logger   *slog.Logger
}

const issueEnrichmentTimeout = 20 * time.Second

func NewIssueEnricher(analyzer *ai.Analyzer, catalog *CatalogService, logger *slog.Logger) *IssueEnricher {
	return &IssueEnricher{
		analyzer: analyzer,
		catalog:  catalog,
		logger:   logger,
	}
}

func (s *IssueEnricher) AnalyzeCreateInput(ctx context.Context, input issues.CreateInput) (issues.CreateInput, error) {
	ctx, cancel := context.WithTimeout(ctx, issueEnrichmentTimeout)
	defer cancel()

	start := time.Now()
	s.logger.InfoContext(ctx, "analyzing create input", "tag_count", len(input.Tags))

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

	s.logger.InfoContext(ctx, "create input analyzed",
		"scored_tags", len(input.TagScores),
		"has_embedding", len(input.Embedding) > 0,
		"duration", time.Since(start).Round(time.Millisecond),
	)
	return input, nil
}

func (s *IssueEnricher) AnalyzeRefineInput(ctx context.Context, issue issues.Issue, postRaw string, createdBy string) (issues.RefineInput, error) {
	ctx, cancel := context.WithTimeout(ctx, issueEnrichmentTimeout)
	defer cancel()

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

func (s *IssueEnricher) AnalyzePersistedIssue(
	ctx context.Context,
	issue issues.Issue,
	targetSequence int,
) (issues.IssueFieldUpdate, error) {
	ctx, cancel := context.WithTimeout(ctx, issueEnrichmentTimeout)
	defer cancel()

	start := time.Now()
	s.logger.InfoContext(ctx, "enriching persisted issue",
		"issue_id", issue.ID,
		"target_sequence", targetSequence,
	)

	targetSequence = max(1, targetSequence)
	discussionTexts := make([]string, 0, len(issue.Discussion))
	for _, post := range issue.Discussion {
		if post.Sequence > targetSequence {
			continue
		}
		text := strings.TrimSpace(post.Raw)
		if text == "" {
			continue
		}
		discussionTexts = append(discussionTexts, text)
	}
	if len(discussionTexts) == 0 {
		text := strings.TrimSpace(issue.Raw)
		if text == "" {
			return issues.IssueFieldUpdate{}, fmt.Errorf("issue raw is required")
		}
		discussionTexts = append(discussionTexts, text)
	}

	explicitTags := []string(nil)
	if targetSequence <= 1 {
		explicitTags = append([]string(nil), issue.Tags...)
	}
	taxonomy, err := s.catalog.IssueTaxonomy(ctx, explicitTags)
	if err != nil {
		return issues.IssueFieldUpdate{}, err
	}

	canonicalRaw := discussionTexts[0]
	if targetSequence > 1 {
		canonicalRaw, err = s.analyzer.CanonicalizeDiscussion(ctx, discussionTexts)
		if err != nil {
			return issues.IssueFieldUpdate{}, err
		}
		canonicalRaw = strings.TrimSpace(canonicalRaw)
		if canonicalRaw == "" {
			return issues.IssueFieldUpdate{}, fmt.Errorf("canonical raw is required")
		}
	}

	analyzed, err := s.analyzer.AnalyzeIssueData(ctx, canonicalRaw, taxonomy)
	if err != nil {
		return issues.IssueFieldUpdate{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, CatalogTagsFromAnalysis(taxonomy, explicitTags, analyzed.Tags)); err != nil {
		return issues.IssueFieldUpdate{}, err
	}

	s.logger.InfoContext(ctx, "persisted issue enriched",
		"issue_id", issue.ID,
		"scored_tags", len(analyzed.Tags),
		"has_embedding", len(analyzed.Embedding.Vector) > 0,
		"duration", time.Since(start).Round(time.Millisecond),
	)

	status := issues.EnrichmentStatusComplete
	emptyError := ""
	return issues.IssueFieldUpdate{
		Raw:                      &canonicalRaw,
		Tags:                     issues.DisplayTags(explicitTags, IssueTagScoresFromAnalysis(analyzed.Tags)),
		TagScores:                IssueTagScoresFromAnalysis(analyzed.Tags),
		Embedding:                Float32VectorToFloat64(analyzed.Embedding.Vector),
		EnrichmentStatus:         &status,
		EnrichmentError:          &emptyError,
		EnrichmentTargetSequence: &targetSequence,
	}, nil
}

func (s *IssueEnricher) AnalyzeCombineInput(
	ctx context.Context,
	sourceIssues []issues.Issue,
	createdBy string,
	note string,
) (issues.CombineInput, error) {
	ctx, cancel := context.WithTimeout(ctx, issueEnrichmentTimeout)
	defer cancel()

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
