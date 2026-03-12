package queries

import (
	"cmp"
	"context"
	"math"
	"slices"

	"splat/internal/issues"
	"splat/internal/vectors"
)

type CompareIssues struct {
	IDs []string
}

type PairwiseIssueSimilarity struct {
	SourceID   string
	TargetID   string
	Similarity float64
}

type CompareIssuesResult struct {
	ComparedIssueCount          int
	AverageEmbeddingSimilarity  float64
	PairwiseEmbeddingSimilarity []PairwiseIssueSimilarity
}

type CompareIssuesHandler struct {
	Store issues.Store
}

type compareIssueLister interface {
	ListCompareIssues(context.Context, []string) ([]issues.CompareIssue, error)
}

func (h CompareIssuesHandler) Handle(ctx context.Context, input CompareIssues) (CompareIssuesResult, error) {
	if h.Store == nil {
		return CompareIssuesResult{}, issues.ErrNotFound
	}

	selected, err := h.compareIssues(ctx, input.IDs)
	if err != nil {
		return CompareIssuesResult{}, err
	}

	pairs, average := compareIssueEmbeddings(selected)
	return CompareIssuesResult{
		ComparedIssueCount:          len(selected),
		AverageEmbeddingSimilarity:  average,
		PairwiseEmbeddingSimilarity: pairs,
	}, nil
}

func (h CompareIssuesHandler) compareIssues(ctx context.Context, ids []string) ([]issues.CompareIssue, error) {
	if compareStore, ok := h.Store.(compareIssueLister); ok {
		return compareStore.ListCompareIssues(ctx, ids)
	}

	items := make([]issues.CompareIssue, 0, len(ids))
	for _, id := range ids {
		issue, err := h.Store.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, issues.CompareIssue{
			ID:        issue.ID,
			Embedding: append([]float64(nil), issue.Embedding...),
		})
	}
	return items, nil
}

func compareIssueEmbeddings(items []issues.CompareIssue) ([]PairwiseIssueSimilarity, float64) {
	pairs := make([]PairwiseIssueSimilarity, 0, len(items)*(len(items)-1)/2)
	total := 0.0

	for i := range items {
		for j := i + 1; j < len(items); j++ {
			similarity := vectors.CosineSimilarity(items[i].Embedding, items[j].Embedding)
			total += similarity
			pairs = append(pairs, PairwiseIssueSimilarity{
				SourceID:   items[i].ID,
				TargetID:   items[j].ID,
				Similarity: math.Round(similarity*100) / 100,
			})
		}
	}

	slices.SortStableFunc(pairs, func(a, b PairwiseIssueSimilarity) int {
		if c := cmp.Compare(b.Similarity, a.Similarity); c != 0 {
			return c
		}
		if c := cmp.Compare(a.SourceID, b.SourceID); c != 0 {
			return c
		}
		return cmp.Compare(a.TargetID, b.TargetID)
	})

	average := 0.0
	if len(pairs) > 0 {
		average = math.Round((total/float64(len(pairs)))*100) / 100
	}
	return pairs, average
}
