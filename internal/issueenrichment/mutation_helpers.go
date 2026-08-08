package enrichment

import (
	"context"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

func (s *IssueEnricher) analyzeMutationText(ctx context.Context, canonicalRaw string, explicitTags []string) (AnalyzeTextResult, error) {
	analysis, err := s.AnalyzeText(ctx, canonicalRaw, AnalyzeTextOptions{
		PreferredTags: explicitTags,
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		return AnalyzeTextResult{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, tags.CatalogTagsFromAnalysis(analysis.CandidateSet.AITags(), explicitTags, analysis.Analyzed.Tags)); err != nil {
		return AnalyzeTextResult{}, err
	}

	return analysis, nil
}

func (s *IssueEnricher) displayTagsForAnalysis(ctx context.Context, tagScores []domain.TagRelevance) []string {
	return issues.DisplayTagsWithSpecificity(nil, tagScores, s.tagSpecificityMap(ctx))
}
