package enrichment

import (
	"context"
	"time"

	"sortit/internal/issues"
)

func (s *IssueEnricher) AnalyzeCreateInput(ctx context.Context, input issues.CreateInput) (issues.CreateInput, error) {
	ctx, cancel := context.WithTimeout(ctx, issueEnrichmentTimeout)
	defer cancel()

	start := time.Now()
	s.logger.InfoContext(ctx, "analyzing create input", "tag_count", len(input.Tags))

	analysis, err := s.analyzeMutationText(ctx, input.Raw, input.Tags)
	if err != nil {
		return issues.CreateInput{}, err
	}

	input.TagScores = analysis.TagScores
	input.Embedding = Float32VectorToFloat64(analysis.Analyzed.Embedding.Vector)
	if len(input.Tags) == 0 {
		input.Tags = s.displayTagsForAnalysis(ctx, analysis.TagScores)
	}

	s.logger.InfoContext(ctx, "create input analyzed",
		"scored_tags", len(input.TagScores),
		"has_embedding", len(input.Embedding) > 0,
		"duration", time.Since(start).Round(time.Millisecond),
	)
	return input, nil
}
