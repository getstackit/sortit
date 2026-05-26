package enrichment

import (
	"context"
	"fmt"
	"strings"

	"sortit/internal/ai"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

func (s *IssueEnricher) analyzeWithCandidateTaxonomy(ctx context.Context, raw string, preferred []string, mode tags.CandidateMode) (ai.AnalyzedIssue, tags.CandidateTaxonomy, error) {
	freshEmbedding, err := s.analyzer.EmbedText(ctx, raw)
	if err != nil {
		return ai.AnalyzedIssue{}, tags.CandidateTaxonomy{}, fmt.Errorf("embed raw for shortlist: %w", err)
	}
	freshEmbeddingVector := Float32VectorToFloat64(freshEmbedding.Vector)

	candidates, err := s.catalog.IssueTaxonomyCandidates(ctx, freshEmbeddingVector, preferred, mode, 15)
	if err != nil {
		return ai.AnalyzedIssue{}, tags.CandidateTaxonomy{}, err
	}
	candidates = s.catalog.AnnotateCandidateHints(ctx, candidates, freshEmbeddingVector, persistedIssueHintLimit)

	var examples []ai.FewShotExample
	if s.exemplars != nil && s.analyzer != nil {
		candidateTagNames := make([]string, 0, len(candidates.Tags))
		for _, tag := range candidates.Tags {
			candidateTagNames = append(candidateTagNames, tag.Name)
		}
		examples = s.exemplars.Select(ctx, s.analyzer, freshEmbeddingVector, candidateTagNames, 3)
	}

	analyzed, err := s.analyzer.AnalyzeIssueData(ctx, raw, candidates.AITags(), examples)
	if err != nil {
		return ai.AnalyzedIssue{}, tags.CandidateTaxonomy{}, err
	}
	return analyzed, candidates, nil
}

func (s *IssueEnricher) tagSpecificityMap(ctx context.Context) map[string]*float64 {
	if s.catalog == nil {
		return nil
	}
	tags, err := s.catalog.StoredTags(ctx)
	if err != nil {
		return nil
	}
	values := make(map[string]*float64, len(tags))
	for i := range tags {
		values[tags[i].Name] = tags[i].Specificity
	}
	return values
}

func (s *IssueEnricher) AnalyzeText(ctx context.Context, raw string, opts AnalyzeTextOptions) (AnalyzeTextResult, error) {
	analyzed, candidates, err := s.analyzeWithCandidateTaxonomy(ctx, raw, opts.PreferredTags, opts.CandidateMode)
	if err != nil {
		return AnalyzeTextResult{}, err
	}

	tagSpecificity := s.tagSpecificityMap(ctx)
	tagScores := attenuateGenericScores(IssueTagScoresFromAnalysis(analyzed.Tags), tagSpecificity)

	tagScores = s.decorateAndVerifyTagScores(
		ctx,
		raw,
		Float32VectorToFloat64(analyzed.Embedding.Vector),
		candidates,
		tagScores,
		analyzed.Tags,
		analyzed.Negated,
		tagSpecificity,
		opts.Verify,
	)

	return AnalyzeTextResult{
		Analyzed:     analyzed,
		CandidateSet: candidates,
		TagScores:    tagScores,
	}, nil
}

func IssueTagScoresFromAnalysis(scores []ai.TagScore) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}

	tagScores := make([]issues.TagRelevance, 0, len(scores))
	for _, score := range scores {
		if score.Tag == "" {
			continue
		}
		if score.Relevance < issueTagRelevanceFloor {
			continue
		}
		tagScores = append(tagScores, issues.TagRelevance{
			Tag:         score.Tag,
			Relevance:   score.Relevance,
			Suggested:   score.Suggested,
			Description: strings.TrimSpace(score.Description),
		})
	}
	return tagScores
}
