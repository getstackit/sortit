package vectors

import (
	"math"

	"gonum.org/v1/gonum/floats"
)

// Mean returns the component-wise mean of the given vectors. The dimension
// is taken from the first non-empty vector; empty vectors and vectors of a
// different dimension are skipped. Returns nil when no usable vector exists.
func Mean(vecs [][]float64) []float64 {
	var sum []float64
	count := 0
	for _, vec := range vecs {
		if len(vec) == 0 {
			continue
		}
		if sum == nil {
			sum = make([]float64, len(vec))
		}
		if len(vec) != len(sum) {
			continue
		}
		floats.Add(sum, vec)
		count++
	}
	if count == 0 {
		return nil
	}
	floats.Scale(1/float64(count), sum)
	return sum
}

// CenterUnit returns a unit-length copy of v with mean subtracted. It falls
// back to a unit-length copy of v (uncentered) when the mean is empty or of
// a different dimension, or when centering yields the zero vector — which
// covers the single-vector corpus, where the mean equals the vector itself.
// A zero input vector is returned as a zero copy, never as -mean.
func CenterUnit(v, mean []float64) []float64 {
	if len(v) == 0 {
		return nil
	}

	out := append([]float64(nil), v...)
	if IsZero(out) {
		return out
	}

	if len(mean) != len(out) {
		NormalizeUnit(out)
		return out
	}

	floats.Sub(out, mean)
	if IsZero(out) {
		copy(out, v)
	}
	NormalizeUnit(out)
	return out
}

// IsZero reports whether every component of values is exactly zero (an empty
// vector counts as zero).
func IsZero(values []float64) bool {
	for _, value := range values {
		if value != 0 {
			return false
		}
	}
	return true
}

// NormalizeUnit scales vector in place to unit length. A zero vector is left
// unchanged.
func NormalizeUnit(vector []float64) {
	// floats.Dot(v, v) gives the sum of squares through the fast unrolled
	// kernel; floats.Norm would be overflow-safe but slower, and unit-length
	// embeddings never approach the overflow regime it guards against.
	sumSq := floats.Dot(vector, vector)
	if sumSq == 0 {
		return
	}
	floats.Scale(1/math.Sqrt(sumSq), vector)
}

// NormalizedCopy returns a unit-length copy of vector, leaving the input
// untouched. A zero or empty vector is returned as an unmodified copy.
func NormalizedCopy(vector []float64) []float64 {
	out := append([]float64(nil), vector...)
	NormalizeUnit(out)
	return out
}
