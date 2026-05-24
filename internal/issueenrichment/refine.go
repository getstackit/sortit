package enrichment

import (
	"context"
	"fmt"
	"strings"

	"sortit/internal/issues"
)

func (s *IssueEnricher) AnalyzeRefineInput(ctx context.Context, issue issues.Issue, postRaw string, createdBy string) (issues.RefineInput, error) {
	ctx, cancel := context.WithTimeout(ctx, issueEnrichmentTimeout)
	defer cancel()

	postRaw = strings.TrimSpace(postRaw)
	if postRaw == "" {
		return issues.RefineInput{}, fmt.Errorf("post raw is required")
	}

	canonicalRaw, err := s.canonicalizeRefinement(ctx, issue, postRaw)
	if err != nil {
		return issues.RefineInput{}, err
	}

	analysis, err := s.analyzeMutationText(ctx, canonicalRaw, nil)
	if err != nil {
		return issues.RefineInput{}, err
	}

	return issues.RefineInput{
		PostRaw:      postRaw,
		CanonicalRaw: canonicalRaw,
		Tags:         s.displayTagsForAnalysis(ctx, analysis.TagScores),
		CreatedBy:    createdBy,
		TagScores:    analysis.TagScores,
		Embedding:    Float32VectorToFloat64(analysis.Analyzed.Embedding.Vector),
	}, nil
}
