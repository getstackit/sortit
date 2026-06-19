package vectors

import (
	"math"

	"gonum.org/v1/gonum/floats"
)

// CosineSimilarity computes the cosine similarity between two vectors.
//
// This is a single fused pass with three independent accumulators (dot, magA,
// magB). On arm64 the Go compiler auto-vectorizes that shape, and it reads each
// vector exactly once — measurably faster than three separate floats kernel
// calls (Dot + two Norms), which would triple the memory traffic.
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
// reduces to a simple dot product. floats.Dot uses an unrolled multi-accumulator
// kernel that beats a naive single-accumulator loop (~2.8x on arm64); this is the
// hottest primitive — it backs the O(N²) edge build, tag covariance, and every
// per-candidate similarity blend.
func UnitCosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	return floats.Dot(a, b)
}
