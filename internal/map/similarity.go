package issuemap

import "math"

type Edge struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Similarity float64 `json:"similarity"`
}

func ComputeEdges(issues []Issue, threshold float64) []Edge {
	embeddings := AllEmbeddings()
	var edges []Edge

	for i := 0; i < len(issues); i++ {
		a := embeddings[issues[i].ID]
		if a == nil {
			continue
		}
		for j := i + 1; j < len(issues); j++ {
			b := embeddings[issues[j].ID]
			if b == nil {
				continue
			}
			sim := unitCosineSimilarity(a, b)
			if sim >= threshold {
				edges = append(edges, Edge{
					Source:     issues[i].ID,
					Target:     issues[j].ID,
					Similarity: math.Round(sim*100) / 100,
				})
			}
		}
	}

	return edges
}

// unitCosineSimilarity assumes both vectors were normalized up front.
func unitCosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}

	var dot float64
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}
