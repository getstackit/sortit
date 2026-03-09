package issuemap

import (
	"math"

	"splat/internal/issues"
)

const embeddingDimensions = 24

type tagSpec struct {
	Name        string
	Description string
}

var tagCatalog = defaultTagCatalog()

func defaultTagCatalog() []tagSpec {
	definitions := issues.DefaultTags()
	specs := make([]tagSpec, len(definitions))
	for i, definition := range definitions {
		specs[i] = tagSpec{
			Name:        definition.Name,
			Description: definition.Description,
		}
	}
	return specs
}

func round(value float64, digits int) float64 {
	scale := math.Pow(10, float64(digits))
	return math.Round(value*scale) / scale
}

func normalizeVector(vector []float64) {
	var magnitude float64
	for _, value := range vector {
		magnitude += value * value
	}
	if magnitude == 0 {
		return
	}

	scale := math.Sqrt(magnitude)
	for i := range vector {
		vector[i] /= scale
	}
}
