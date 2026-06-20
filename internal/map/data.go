package issuemap

import (
	"math"

	"gonum.org/v1/gonum/floats"

	"sortit/internal/issues"
)

const embeddingDimensions = 64

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

func round(value float64) float64 {
	return math.Round(value*100) / 100
}

func normalizeVector(vector []float64) {
	// floats.Dot(v, v) is the sum of squares through the unrolled kernel.
	magnitude := floats.Dot(vector, vector)
	if magnitude == 0 {
		return
	}
	floats.Scale(1/math.Sqrt(magnitude), vector)
}
