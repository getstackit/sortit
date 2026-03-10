package queries

import (
	"context"

	"splat/internal/issues"
)

type semanticIssueSearcher interface {
	SearchIssues(context.Context, issues.SemanticSearchOptions) ([]issues.SemanticSearchResult, error)
}

func semanticSearchStore(store issues.Store) (semanticIssueSearcher, bool) {
	searcher, ok := store.(semanticIssueSearcher)
	return searcher, ok
}

func semanticSearchCandidateIssues(
	ctx context.Context,
	searcher semanticIssueSearcher,
	opts issues.SemanticSearchOptions,
) ([]issues.Issue, error) {
	results, err := searcher.SearchIssues(ctx, opts)
	if err != nil {
		return nil, err
	}

	items := make([]issues.Issue, 0, len(results))
	for _, result := range results {
		items = append(items, result.Issue)
	}
	return items, nil
}

func semanticSearchCandidateLimit(limit, offset int) int {
	if limit <= 0 {
		limit = 8
	}
	if offset < 0 {
		offset = 0
	}

	window := (limit + offset) * 6
	if window < 32 {
		window = 32
	}
	if window > 256 {
		window = 256
	}
	if window < limit+offset {
		window = limit + offset
	}
	return window
}
