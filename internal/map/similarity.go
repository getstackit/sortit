package issuemap

import (
	"math"

	"splat/internal/issues"
	"splat/internal/vectors"
)

type Edge struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Similarity float64 `json:"similarity"`
}

func ComputeEdgesWithEmbeddings(items []issues.Issue, embeddings map[string][]float64, threshold float64) []Edge {
	var edges []Edge

	for i := range items {
		a := embeddings[items[i].ID]
		if a == nil {
			continue
		}
		for j := i + 1; j < len(items); j++ {
			b := embeddings[items[j].ID]
			if b == nil {
				continue
			}
			sim := vectors.UnitCosineSimilarity(a, b)
			if sim >= threshold {
				edges = append(edges, Edge{
					Source:     items[i].ID,
					Target:     items[j].ID,
					Similarity: math.Round(sim*100) / 100,
				})
			}
		}
	}

	return edges
}

