package issuemath

import (
	"slices"

	"sortit/internal/scoring"
	"sortit/internal/vectors"
)

// CorpusMeans holds the corpus-level embedding means used for runtime
// centering. OpenAI text-embedding vectors over a topically homogeneous
// corpus share a large common direction (anisotropy), which inflates every
// cosine the math layer computes. Subtracting the corpus mean and
// re-normalizing removes that shared component. The means are a runtime
// transform — persisted embeddings are never rewritten.
//
// Issue is the mean of the issue-corpus embeddings; Tag is the mean of the
// tag-catalog embeddings. They are kept separate because the two populations
// have different common directions.
type CorpusMeans struct {
	Issue []float64
	Tag   []float64
}

// IsZero reports whether no means are available (empty corpus and catalog).
func (m CorpusMeans) IsZero() bool {
	return len(m.Issue) == 0 && len(m.Tag) == 0
}

// ComputeCorpusMeans computes the issue-corpus and tag-catalog embedding
// means. Keys are visited in sorted order so the result is deterministic.
// Either mean is nil when its population has no usable vectors or fewer than
// scoring.MinCenteringVectors of them — a mean over a handful of vectors is
// noise, and subtracting it makes near-parallel vectors antipodal.
func ComputeCorpusMeans(issueEmbeddings, tagEmbeddings map[string][]float64) CorpusMeans {
	return CorpusMeans{
		Issue: populationMean(issueEmbeddings),
		Tag:   populationMean(tagEmbeddings),
	}
}

func populationMean(embeddings map[string][]float64) []float64 {
	values := sortedValues(embeddings)
	usable := 0
	for _, vec := range values {
		if len(vec) > 0 {
			usable++
		}
	}
	if usable < scoring.MinCenteringVectors {
		return nil
	}
	return vectors.Mean(values)
}

// CenterEmbeddings computes the corpus means and returns centered,
// unit-normalized copies of both embedding sets along with the means used.
// The inputs are not mutated.
func CenterEmbeddings(issueEmbeddings, tagEmbeddings map[string][]float64) (map[string][]float64, map[string][]float64, CorpusMeans) {
	means := ComputeCorpusMeans(issueEmbeddings, tagEmbeddings)
	centeredIssues, centeredTags := CenterEmbeddingsWith(means, issueEmbeddings, tagEmbeddings)
	return centeredIssues, centeredTags, means
}

// CenterEmbeddingsWith centers both embedding sets against externally
// supplied means (e.g. revision-cached corpus means, so a retrieved subset
// is centered consistently with the full corpus). Vectors that would become
// zero after centering — including the single-vector corpus — fall back to
// their uncentered (unit-normalized) form. The inputs are not mutated.
func CenterEmbeddingsWith(means CorpusMeans, issueEmbeddings, tagEmbeddings map[string][]float64) (map[string][]float64, map[string][]float64) {
	return centerSet(issueEmbeddings, means.Issue), centerSet(tagEmbeddings, means.Tag)
}

// CenterVector centers a single vector (e.g. a search-query embedding)
// against the given mean, with the same fallbacks as CenterEmbeddingsWith.
func CenterVector(v, mean []float64) []float64 {
	return vectors.CenterUnit(v, mean)
}

func centerSet(embeddings map[string][]float64, mean []float64) map[string][]float64 {
	if embeddings == nil {
		return nil
	}
	out := make(map[string][]float64, len(embeddings))
	for key, vec := range embeddings {
		out[key] = vectors.CenterUnit(vec, mean)
	}
	return out
}

func sortedValues(embeddings map[string][]float64) [][]float64 {
	if len(embeddings) == 0 {
		return nil
	}
	keys := make([]string, 0, len(embeddings))
	for key := range embeddings {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make([][]float64, len(keys))
	for i, key := range keys {
		out[i] = embeddings[key]
	}
	return out
}
