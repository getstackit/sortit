package mapview

import (
	"context"
	"errors"
	"strings"
	"time"

	"sortit/internal/issues"
	issueviews "sortit/internal/issues/views"
	issuemap "sortit/internal/map"
	"sortit/internal/ridgelambda"
	"sortit/internal/tags"
)

type ExploreIssue struct {
	ID    string
	Limit int
}

type ExploreIssueHandler struct {
	Reader       issues.Reader
	DetailReader issues.IssueDetailReader
	SearchStore  issues.SemanticSearchStore
	Catalog      *tags.CatalogService
	// RidgeLambda selects the anchored-ridge similarity model for explore
	// when wired and the corpus is large enough; otherwise explore uses the
	// rank-1 factor model. Mirrors the search path.
	RidgeLambda *ridgelambda.Cache
}

// exploreOpts returns the ridge-similarity option carrying the
// revision-cached GCV penalty, or nil when ridge is unavailable.
func (h ExploreIssueHandler) exploreOpts(ctx context.Context) ([]issuemap.ExploreOption, error) {
	if h.RidgeLambda == nil {
		return nil, nil
	}
	lambda, ok, err := h.RidgeLambda.Current(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return []issuemap.ExploreOption{issuemap.WithExploreRidgeSimilarity(lambda)}, nil
}

func (h ExploreIssueHandler) Handle(ctx context.Context, input ExploreIssue) (issuemap.ExploreResponse, error) {
	if h.SearchStore != nil {
		return h.handleSemanticExplore(ctx, input, h.SearchStore)
	}

	storeIssues, err := h.Reader.List(ctx)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}
	if h.DetailReader != nil {
		storeIssues = issueviews.HydrateIssuesWithVelocity(ctx, h.DetailReader, storeIssues, time.Now().UTC())
	}

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}

	ridgeOpts, err := h.exploreOpts(ctx)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}

	return issuemap.ExploreFromIssuesWithTags(storeIssues, storeTags, input.ID, input.Limit, ridgeOpts...)
}

func (h ExploreIssueHandler) handleSemanticExplore(
	ctx context.Context,
	input ExploreIssue,
	searcher semanticIssueSearcher,
) (issuemap.ExploreResponse, error) {
	target, err := h.DetailReader.GetIssueDetail(ctx, input.ID)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}
	target = issueviews.HydrateIssueWithVelocity(ctx, nil, target, time.Now().UTC())

	storeTags, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}

	candidateSet, err := exploreSemanticCandidateSet(ctx, h.DetailReader, searcher, target, input.Limit)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}

	ridgeOpts, err := h.exploreOpts(ctx)
	if err != nil {
		return issuemap.ExploreResponse{}, err
	}

	return issuemap.ExploreFromIssuesWithTags(candidateSet, storeTags, target.ID, input.Limit, ridgeOpts...)
}

func exploreSemanticCandidateSet(
	ctx context.Context,
	reader issues.IssueDetailReader,
	searcher semanticIssueSearcher,
	target issues.Issue,
	limit int,
) ([]issues.Issue, error) {
	results, err := searcher.SearchIssues(ctx, issues.SemanticSearchOptions{
		QueryEmbedding: target.Embedding,
		Status:         issues.StatusOpen,
		ExcludeID:      target.ID,
		Limit:          semanticSearchCandidateLimit(limit, 0),
	})
	if err != nil {
		return nil, err
	}

	items := []issues.Issue{target}
	seen := map[string]struct{}{target.ID: {}}

	now := time.Now().UTC()
	addDetailedIssue := func(id string, fallback *issues.Issue) error {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil
		}
		if _, ok := seen[id]; ok {
			return nil
		}

		issue, err := reader.GetIssueDetail(ctx, id)
		if err != nil {
			switch {
			case fallback != nil && errors.Is(err, issues.ErrNotFound):
				issue = *fallback
			case errors.Is(err, issues.ErrNotFound):
				return nil
			default:
				return err
			}
		}
		issue = issueviews.HydrateIssueWithVelocity(ctx, nil, issue, now)

		seen[issue.ID] = struct{}{}
		items = append(items, issue)
		return nil
	}

	for _, result := range results {
		resultIssue := result.Issue
		if err := addDetailedIssue(resultIssue.ID, &resultIssue); err != nil {
			return nil, err
		}
	}

	for _, relatedID := range directExploreSupplementIssueIDs(target) {
		if err := addDetailedIssue(relatedID, nil); err != nil {
			return nil, err
		}
	}

	for i := 0; i < len(items); i++ {
		for _, canonicalID := range canonicalExploreTargetIDs(items[i]) {
			if err := addDetailedIssue(canonicalID, nil); err != nil {
				return nil, err
			}
		}
	}

	return items, nil
}

func directExploreSupplementIssueIDs(target issues.Issue) []string {
	ids := make([]string, 0, len(target.Links))
	seen := make(map[string]struct{}, len(target.Links))

	for _, link := range target.Links {
		switch link.Type {
		case issues.IssueLinkTypeRelatedTo,
			issues.IssueLinkTypeDerivedFrom,
			issues.IssueLinkTypeParentOf,
			issues.IssueLinkTypeChildOf:
			if link.SourceIssueID == target.ID {
				if addExploreIssueID(seen, link.TargetIssueID) {
					ids = append(ids, strings.TrimSpace(link.TargetIssueID))
				}
			} else if link.TargetIssueID == target.ID {
				if addExploreIssueID(seen, link.SourceIssueID) {
					ids = append(ids, strings.TrimSpace(link.SourceIssueID))
				}
			}
		case issues.IssueLinkTypeMergedInto, issues.IssueLinkTypeDuplicateOf:
			if link.SourceIssueID == target.ID && addExploreIssueID(seen, link.TargetIssueID) {
				ids = append(ids, strings.TrimSpace(link.TargetIssueID))
			}
		}
	}

	return ids
}

func canonicalExploreTargetIDs(item issues.Issue) []string {
	ids := make([]string, 0, len(item.Links))
	seen := make(map[string]struct{}, len(item.Links))

	for _, link := range item.Links {
		if link.SourceIssueID != item.ID {
			continue
		}
		if link.Type != issues.IssueLinkTypeMergedInto && link.Type != issues.IssueLinkTypeDuplicateOf {
			continue
		}
		if addExploreIssueID(seen, link.TargetIssueID) {
			ids = append(ids, strings.TrimSpace(link.TargetIssueID))
		}
	}

	return ids
}

func addExploreIssueID(seen map[string]struct{}, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if _, ok := seen[id]; ok {
		return false
	}
	seen[id] = struct{}{}
	return true
}

type semanticIssueSearcher interface {
	SearchIssues(context.Context, issues.SemanticSearchOptions) ([]issues.SemanticSearchResult, error)
}

func semanticSearchCandidateLimit(limit, offset int) int {
	if limit <= 0 {
		limit = 8
	}
	if offset < 0 {
		offset = 0
	}

	window := max((limit+offset)*6, 32)
	if window > 256 {
		window = 256
	}
	if window < limit+offset {
		window = limit + offset
	}
	return window
}
