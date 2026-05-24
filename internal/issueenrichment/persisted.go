package enrichment

import (
	"context"
	"time"

	"sortit/internal/issues"
)

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

	canonicalRaw, targetSequence, err := s.canonicalizePersistedIssue(ctx, issue, targetSequence)
	if err != nil {
		return issues.IssueFieldUpdate{}, err
	}

	analysis, err := s.analyzeMutationText(ctx, canonicalRaw, nil)
	if err != nil {
		return issues.IssueFieldUpdate{}, err
	}

	s.logger.InfoContext(ctx, "persisted issue enriched",
		"issue_id", issue.ID,
		"scored_tags", len(analysis.Analyzed.Tags),
		"has_embedding", len(analysis.Analyzed.Embedding.Vector) > 0,
		"duration", time.Since(start).Round(time.Millisecond),
	)

	status := issues.EnrichmentStatusComplete
	emptyError := ""

	return issues.IssueFieldUpdate{
		Raw:                      &canonicalRaw,
		Tags:                     s.displayTagsForAnalysis(ctx, analysis.TagScores),
		TagScores:                analysis.TagScores,
		Embedding:                Float32VectorToFloat64(analysis.Analyzed.Embedding.Vector),
		EnrichmentStatus:         &status,
		EnrichmentError:          &emptyError,
		EnrichmentTargetSequence: &targetSequence,
	}, nil
}
