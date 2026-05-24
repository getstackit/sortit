package vectors

import "math"

// CosineSimilarity computes the cosine similarity between two vectors.
func CosineSimilarity(a, b []float64) float64 {
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

// UnitCosineSimilarity computes the cosine similarity between two vectors that
// have already been normalized to unit length. Since the magnitudes are 1, this
// reduces to a simple dot product.
func UnitCosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}

	var dot float64
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot
}
