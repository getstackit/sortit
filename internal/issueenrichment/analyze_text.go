package enrichment

import (
	"context"
	"fmt"
	"strings"

	"sortit/internal/ai"
	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

func (s *IssueEnricher) analyzeWithCandidateTaxonomy(ctx context.Context, raw string, preferred []string, mode tags.CandidateMode, includePriorDecisions bool) (ai.AnalyzedIssue, tags.CandidateTaxonomy, error) {
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

	var priorDecisions []ai.PriorDecision
	if includePriorDecisions {
		priorDecisions = s.relevantPriorDecisions(ctx, freshEmbeddingVector)
	}

	analyzed, err := s.analyzer.AnalyzeIssueData(ctx, raw, candidates.AITags(), examples, ai.ConceptFrame{}, priorDecisions...)
	if err != nil {
		return ai.AnalyzedIssue{}, tags.CandidateTaxonomy{}, err
	}
	return analyzed, candidates, nil
}

// EmbedText returns the float64 embedding for raw text — the same embedding the
// enrichment pipeline computes for shortlisting and memory context. It lets
// callers (e.g. query-time memory recall) reuse the configured embedder without
// running a full tag analysis.
func (s *IssueEnricher) EmbedText(ctx context.Context, raw string) ([]float64, error) {
	if s.analyzer == nil {
		return nil, fmt.Errorf("embed text: analyzer not configured")
	}
	result, err := s.analyzer.EmbedText(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("embed text: %w", err)
	}
	return Float32VectorToFloat64(result.Vector), nil
}

// relevantPriorDecisions retrieves the memories most similar to the text being
// enriched and shapes them as tagging context, so a new issue restating a
// settled decision is tagged consistently with it. Failures are non-fatal.
func (s *IssueEnricher) relevantPriorDecisions(ctx context.Context, embedding []float64) []ai.PriorDecision {
	if s.memoryStore == nil || len(embedding) == 0 {
		return nil
	}
	results, err := s.memoryStore.SearchMemories(ctx, embedding, memoryContextLimit)
	if err != nil {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "memory context retrieval failed", "error", err)
		}
		return nil
	}
	decisions := make([]ai.PriorDecision, 0, len(results))
	for _, result := range results {
		if result.Similarity < memoryContextSimFloor {
			continue
		}
		decisions = append(decisions, ai.PriorDecision{
			Title:   priorDecisionTitle(result.Memory),
			Summary: result.Memory.Body,
			Tags:    result.Memory.AnchorTags,
		})
	}
	return decisions
}

func priorDecisionTitle(memory domain.Memory) string {
	if title := strings.TrimSpace(memory.Title); title != "" {
		return title
	}
	if len(memory.AnchorTags) > 0 {
		return memory.AnchorTags[0]
	}
	return "memory"
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
	analyzed, candidates, err := s.analyzeWithCandidateTaxonomy(ctx, raw, opts.PreferredTags, opts.CandidateMode, !opts.SkipPriorDecisions)
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
