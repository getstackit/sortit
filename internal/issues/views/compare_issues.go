package issueviews

import (
	"cmp"
	"context"
	"math"
	"slices"

	"sortit/internal/issues"
	"sortit/internal/vectors"
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
	Reader issues.CompareIssueReader
}

func (h CompareIssuesHandler) Handle(ctx context.Context, input CompareIssues) (CompareIssuesResult, error) {
	if h.Reader == nil {
		return CompareIssuesResult{}, issues.ErrNotFound
	}

	selected, err := h.Reader.ListCompareIssues(ctx, input.IDs)
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
