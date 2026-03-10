package queries

import (
	"context"
	"math"
	"sort"

	"splat/internal/issues"
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

func (h CompareIssuesHandler) Handle(ctx context.Context, input CompareIssues) (CompareIssuesResult, error) {
	selected := make([]issues.Issue, 0, len(input.IDs))
	if h.Store != nil {
		for _, id := range input.IDs {
			issue, err := h.Store.Get(ctx, id)
			if err != nil {
				return CompareIssuesResult{}, err
			}
			selected = append(selected, issue)
		}
	} else {
		return CompareIssuesResult{}, issues.ErrNotFound
	}

	pairs, average := compareIssueEmbeddings(selected)
	return CompareIssuesResult{
		ComparedIssueCount:          len(selected),
		AverageEmbeddingSimilarity:  average,
		PairwiseEmbeddingSimilarity: pairs,
	}, nil
}

func compareIssueEmbeddings(items []issues.Issue) ([]PairwiseIssueSimilarity, float64) {
	pairs := make([]PairwiseIssueSimilarity, 0, len(items)*(len(items)-1)/2)
	total := 0.0

	for i := range items {
		for j := i + 1; j < len(items); j++ {
			similarity := CosineSimilarity(items[i].Embedding, items[j].Embedding)
			total += similarity
			pairs = append(pairs, PairwiseIssueSimilarity{
				SourceID:   items[i].ID,
				TargetID:   items[j].ID,
				Similarity: math.Round(similarity*100) / 100,
			})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Similarity == pairs[j].Similarity {
			if pairs[i].SourceID == pairs[j].SourceID {
				return pairs[i].TargetID < pairs[j].TargetID
			}
			return pairs[i].SourceID < pairs[j].SourceID
		}
		return pairs[i].Similarity > pairs[j].Similarity
	})

	average := 0.0
	if len(pairs) > 0 {
		average = math.Round((total/float64(len(pairs)))*100) / 100
	}
	return pairs, average
}
