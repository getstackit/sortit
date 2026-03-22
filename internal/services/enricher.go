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

	input.TagScores = attenuateGenericScores(IssueTagScoresFromAnalysis(analyzed.Tags), s.tagSpecificityMap(ctx))
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
		TagScores:    attenuateGenericScores(IssueTagScoresFromAnalysis(analyzed.Tags), s.tagSpecificityMap(ctx)),
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

	// Always use the full catalog taxonomy so the AI can discover better tags.
	// For initial creation (targetSequence <= 1), we used to pass only the
	// issue's explicit tags as preferred, but that prevented the AI from
	// seeing the rest of the catalog during re-enrichment.
	taxonomy, err := s.catalog.IssueTaxonomy(ctx, nil)
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

	// Mark taxonomy tags that are semantically similar to the issue as hints,
	// so the AI pays closer attention to tags that embedding similarity
	// suggests are relevant but might otherwise be overlooked.
	taxonomy = s.catalog.AnnotateHints(ctx, taxonomy, issue.Embedding, 5)

	analyzed, err := s.analyzer.AnalyzeIssueData(ctx, canonicalRaw, taxonomy)
	if err != nil {
		return issues.IssueFieldUpdate{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, CatalogTagsFromAnalysis(taxonomy, nil, analyzed.Tags)); err != nil {
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
		Tags:                     issues.DisplayTagsWithSpecificity(nil, attenuateGenericScores(IssueTagScoresFromAnalysis(analyzed.Tags), s.tagSpecificityMap(ctx)), s.tagSpecificityMap(ctx)),
		TagScores:                attenuateGenericScores(IssueTagScoresFromAnalysis(analyzed.Tags), s.tagSpecificityMap(ctx)),
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
		TagScores: attenuateGenericScores(IssueTagScoresFromAnalysis(analyzed.Tags), s.tagSpecificityMap(ctx)),
		Embedding: Float32VectorToFloat64(analyzed.Embedding.Vector),
	}, nil
}

const (
	genericSpecificityThreshold = 0.3
	genericMultiplier           = 0.6
	defaultSpecificity          = 0.5
)

// attenuateGenericScores reduces relevance of low-specificity tags when more
// specific tags are also present, so that specific tags rank higher.
// tagSpecificity maps tag name → specificity score (nil = unscored, treated as 0.5).
func attenuateGenericScores(scores []issues.TagRelevance, tagSpecificity map[string]*float64) []issues.TagRelevance {
	if len(scores) == 0 {
		return scores
	}

	hasSpecific := false
	for _, s := range scores {
		if tagSpecificityValue(s.Tag, tagSpecificity) >= genericSpecificityThreshold && s.Relevance > 0 {
			hasSpecific = true
			break
		}
	}
	if !hasSpecific {
		return scores
	}

	out := make([]issues.TagRelevance, len(scores))
	for i, s := range scores {
		out[i] = s
		if tagSpecificityValue(s.Tag, tagSpecificity) < genericSpecificityThreshold {
			out[i].Relevance = s.Relevance * genericMultiplier
		}
	}
	return out
}

func tagSpecificityValue(name string, specificity map[string]*float64) float64 {
	if specificity == nil {
		return defaultSpecificity
	}
	p, ok := specificity[name]
	if !ok || p == nil {
		return defaultSpecificity
	}
	return *p
}

func (s *IssueEnricher) tagSpecificityMap(ctx context.Context) map[string]*float64 {
	if s.catalog == nil {
		return nil
	}
	tags, err := s.catalog.StoredTags(ctx)
	if err != nil {
		return nil
	}
	m := make(map[string]*float64, len(tags))
	for i := range tags {
		m[tags[i].Name] = tags[i].Specificity
	}
	return m
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
