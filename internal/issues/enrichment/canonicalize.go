package enrichment

import (
	"context"
	"fmt"
	"strings"

	"splat/internal/issues"
)

func (s *IssueEnricher) canonicalizeRefinement(ctx context.Context, issue issues.Issue, postRaw string) (string, error) {
	discussionTexts := make([]string, 0, len(issue.Discussion)+1)
	for _, post := range issue.Discussion {
		text := strings.TrimSpace(post.Raw)
		if text == "" {
			continue
		}
		discussionTexts = append(discussionTexts, text)
	}
	discussionTexts = append(discussionTexts, postRaw)

	return s.canonicalizeDiscussion(ctx, discussionTexts)
}

func (s *IssueEnricher) canonicalizePersistedIssue(ctx context.Context, issue issues.Issue, targetSequence int) (string, int, error) {
	targetSequence = max(1, targetSequence)

	discussionTexts, err := persistedIssueDiscussionTexts(issue, targetSequence)
	if err != nil {
		return "", 0, err
	}
	if targetSequence == 1 {
		return discussionTexts[0], targetSequence, nil
	}

	canonicalRaw, err := s.canonicalizeDiscussion(ctx, discussionTexts)
	if err != nil {
		return "", 0, err
	}
	return canonicalRaw, targetSequence, nil
}

func persistedIssueDiscussionTexts(issue issues.Issue, targetSequence int) ([]string, error) {
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
	if len(discussionTexts) > 0 {
		return discussionTexts, nil
	}

	text := strings.TrimSpace(issue.Raw)
	if text == "" {
		return nil, fmt.Errorf("issue raw is required")
	}
	return []string{text}, nil
}

func (s *IssueEnricher) canonicalizeCombinedIssues(ctx context.Context, sourceIssues []issues.Issue, note string) (string, []string, error) {
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

	canonicalRaw, err := s.canonicalizeDiscussion(ctx, parts)
	if err != nil {
		return "", nil, err
	}
	return canonicalRaw, sourceIDs, nil
}

func (s *IssueEnricher) canonicalizeDiscussion(ctx context.Context, discussionTexts []string) (string, error) {
	canonicalRaw, err := s.analyzer.CanonicalizeDiscussion(ctx, discussionTexts)
	if err != nil {
		return "", err
	}
	canonicalRaw = strings.TrimSpace(canonicalRaw)
	if canonicalRaw == "" {
		return "", fmt.Errorf("canonical raw is required")
	}
	return canonicalRaw, nil
}
