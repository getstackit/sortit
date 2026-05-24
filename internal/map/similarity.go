package issuemap

import (
	"math"

	"sortit/internal/issues"
	"sortit/internal/vectors"
)

type Edge struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Similarity float64 `json:"similarity"`
}

func ComputeEdgesWithEmbeddings(items []issues.Issue, embeddings map[string][]float64, threshold float64) []Edge {
	// Pre-extract embeddings into indexed slice to avoid O(n²) hash lookups.
	indexed := make([][]float64, len(items))
	for i := range items {
		indexed[i] = embeddings[items[i].ID]
	}

	var edges []Edge
	for i := range items {
		a := indexed[i]
		if a == nil {
			continue
		}
		for j := i + 1; j < len(items); j++ {
			b := indexed[j]
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
