package vectors

import "math"

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
		for i, v := range vec {
			sum[i] += v
		}
		count++
	}
	if count == 0 {
		return nil
	}
	scale := 1 / float64(count)
	for i := range sum {
		sum[i] *= scale
	}
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
	if isZero(out) {
		return out
	}

	if len(mean) != len(out) {
		normalizeUnit(out)
		return out
	}

	for i := range out {
		out[i] -= mean[i]
	}
	if isZero(out) {
		copy(out, v)
	}
	normalizeUnit(out)
	return out
}

func isZero(values []float64) bool {
	for _, value := range values {
		if value != 0 {
			return false
		}
	}
	return true
}

func normalizeUnit(vector []float64) {
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
