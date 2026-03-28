package issuemath

import (
	"cmp"
	"slices"

	"splat/internal/domain"
	"splat/internal/issues"
)

const minSpecificityCatalogSize = 4

func ComputeTagEmbeddingSpecificity(tags []issues.Tag) map[string]*float64 {
	type scoredTag struct {
		name      string
		embedding []float64
	}

	scored := make([]scoredTag, 0, len(tags))
	for _, tag := range tags {
		name := domain.NormalizeTagName(tag.Name)
		if name == "" || len(tag.Embedding) == 0 {
			continue
		}
		scored = append(scored, scoredTag{
			name:      name,
			embedding: append([]float64(nil), tag.Embedding...),
		})
	}
	slices.SortStableFunc(scored, func(a, b scoredTag) int {
		return cmp.Compare(a.name, b.name)
	})

	scores := make(map[string]*float64, len(scored))
	if len(scored) < minSpecificityCatalogSize {
		return scores
	}

	embeddings := make([][]float64, len(scored))
	for i, tag := range scored {
		embeddings[i] = tag.embedding
	}

	results := NeighborhoodSpecificity(embeddings)
	for i, result := range results {
		score := result.Specificity
		scores[scored[i].name] = &score
	}
	return scores
}
