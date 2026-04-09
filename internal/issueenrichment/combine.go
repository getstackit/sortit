package enrichment

import (
	"context"
	"fmt"

	"sortit/internal/issues"
)

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

	canonicalRaw, sourceIDs, err := s.canonicalizeCombinedIssues(ctx, sourceIssues, note)
	if err != nil {
		return issues.CombineInput{}, err
	}

	analysis, err := s.analyzeMutationText(ctx, canonicalRaw, nil)
	if err != nil {
		return issues.CombineInput{}, err
	}

	return issues.CombineInput{
		SourceIDs: sourceIDs,
		Raw:       canonicalRaw,
		Tags:      s.displayTagsForAnalysis(ctx, analysis.TagScores),
		CreatedBy: createdBy,
		Note:      note,
		TagScores: analysis.TagScores,
		Embedding: Float32VectorToFloat64(analysis.Analyzed.Embedding.Vector),
	}, nil
}
