package enrichment

import (
	"context"

	"splat/internal/issues"
	"splat/internal/services"
)

func (s *IssueEnricher) analyzeMutationText(ctx context.Context, canonicalRaw string, explicitTags []string) (AnalyzeTextResult, error) {
	analysis, err := s.AnalyzeText(ctx, canonicalRaw, AnalyzeTextOptions{
		PreferredTags: explicitTags,
		CandidateMode: services.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		return AnalyzeTextResult{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, services.CatalogTagsFromAnalysis(analysis.CandidateSet.AITags(), explicitTags, analysis.Analyzed.Tags)); err != nil {
		return AnalyzeTextResult{}, err
	}

	return analysis, nil
}

func (s *IssueEnricher) displayTagsForAnalysis(ctx context.Context, tagScores []issues.TagRelevance) []string {
	return issues.DisplayTagsWithSpecificity(nil, tagScores, s.tagSpecificityMap(ctx))
}
