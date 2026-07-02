package issuemap

import (
	"math"

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
