package vectors

import (
	"cmp"
	"slices"
)

// NeighborhoodSpecificityResult holds the intermediate local-density value and
// the final catalog-relative specificity score for a single embedding.
type NeighborhoodSpecificityResult struct {
	MeanNeighborDistance float64
	Specificity          float64
}

// NeighborhoodSpecificity computes a deterministic, catalog-relative
// specificity score for each embedding.
//
// For each embedding, it computes the mean cosine distance to the nearest
// semantic neighbors, then percentile-ranks that mean distance across the
// catalog. Higher scores indicate sparser local neighborhoods and therefore
// more specific semantics.
func NeighborhoodSpecificity(embeddings [][]float64) []NeighborhoodSpecificityResult {
	n := len(embeddings)
	if n == 0 {
		return nil
	}

	neighborCount := min(8, n-1)
	results := make([]NeighborhoodSpecificityResult, n)
	if neighborCount <= 0 {
		return results
	}

	for i, embedding := range embeddings {
		distances := make([]float64, 0, n-1)
		for j, other := range embeddings {
			if i == j {
				continue
			}
			distances = append(distances, cosineDistance(embedding, other))
		}
		slices.Sort(distances)

		var total float64
		for _, distance := range distances[:neighborCount] {
			total += distance
		}
		results[i].MeanNeighborDistance = total / float64(neighborCount)
	}

	assignPercentileSpecificity(results)
	return results
}

func assignPercentileSpecificity(results []NeighborhoodSpecificityResult) {
	if len(results) == 0 {
		return
	}
	if len(results) == 1 {
		results[0].Specificity = 0
		return
	}

	type rankedDistance struct {
		index int
		value float64
	}

	ranked := make([]rankedDistance, len(results))
	for i, result := range results {
		ranked[i] = rankedDistance{index: i, value: result.MeanNeighborDistance}
	}
	slices.SortStableFunc(ranked, func(a, b rankedDistance) int {
		return cmp.Compare(a.value, b.value)
	})

	denominator := float64(len(results) - 1)
	for start := 0; start < len(ranked); {
		end := start + 1
		for end < len(ranked) && ranked[end].value == ranked[start].value {
			end++
		}

		averageRank := float64(start+end-1) / 2
		specificity := averageRank / denominator
		for _, item := range ranked[start:end] {
			results[item.index].Specificity = specificity
		}
		start = end
	}
}

func cosineDistance(a, b []float64) float64 {
	sim := CosineSimilarity(a, b)
	return 1 - sim
}
