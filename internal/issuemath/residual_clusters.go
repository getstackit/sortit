package issuemath

import (
	"cmp"
	"slices"
	"strings"

	"sortit/internal/issues"
	"sortit/internal/vectors"
)

// Residual-cluster thresholds, lifted from the debug R² endpoint's hardcoded
// values so concept mining and the debug view agree on what counts as a
// poorly-explained issue and a shared residual direction.
const (
	// ResidualLowR2Threshold: an issue whose R² falls below this is poorly
	// explained by the tag-factor model — a candidate carrier of an unnamed
	// concept.
	ResidualLowR2Threshold = 0.15
	// ResidualNeighborSimThreshold: two issues share an unexplained concept when
	// their residual directions are at least this cosine-similar.
	ResidualNeighborSimThreshold = 0.3
)

// ResidualNeighbor is another issue whose embedding residual points in a similar
// direction — it shares whatever concept the tags are failing to explain.
type ResidualNeighbor struct {
	ID         string
	Similarity float64
}

// ResidualNeighbors returns the issues whose residual direction is more than
// minSim cosine-similar to the given issue's residual, nearest first (ties broken
// by ID for determinism). The issue itself and any issue without a residual are
// excluded; nil is returned when the issue was not decomposed or has no residual.
// This is the shared primitive behind both the debug R² endpoint's residual
// neighbors and residual-cluster concept mining.
func (d FactorDecomposition) ResidualNeighbors(id string, minSim float64) []ResidualNeighbor {
	if !d.used {
		return nil
	}
	idx, ok := d.index[id]
	if !ok {
		return nil
	}
	residual := d.residuals[idx]
	if len(residual) == 0 {
		return nil
	}

	neighbors := make([]ResidualNeighbor, 0)
	for otherID, otherIdx := range d.index {
		if otherID == id {
			continue
		}
		other := d.residuals[otherIdx]
		if len(other) == 0 {
			continue
		}
		sim := vectors.CosineSimilarity(residual, other)
		if sim > minSim {
			neighbors = append(neighbors, ResidualNeighbor{ID: otherID, Similarity: sim})
		}
	}
	slices.SortFunc(neighbors, func(a, b ResidualNeighbor) int {
		if c := cmp.Compare(b.Similarity, a.Similarity); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return neighbors
}

// TagDataFromIssues collects the deduplicated, sorted tag names referenced by the
// catalog and the given issues, plus the embedding for each catalog tag that has
// one. It is the shared input builder for the factor decomposition — used by the
// debug R² endpoint and residual-cluster concept mining alike.
func TagDataFromIssues(items []issues.Issue, storeTags []issues.Tag) ([]string, map[string][]float64) {
	seen := make(map[string]struct{})
	tagNames := make([]string, 0, len(storeTags))

	for _, tag := range storeTags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		tagNames = append(tagNames, name)
	}
	for _, issue := range items {
		for _, ts := range issue.TagScores {
			name := strings.TrimSpace(ts.Tag)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			tagNames = append(tagNames, name)
		}
	}
	slices.Sort(tagNames)

	tagEmbeddings := make(map[string][]float64, len(storeTags))
	for _, tag := range storeTags {
		name := strings.TrimSpace(tag.Name)
		if name != "" && len(tag.Embedding) > 0 {
			tagEmbeddings[name] = tag.Embedding
		}
	}

	return tagNames, tagEmbeddings
}
