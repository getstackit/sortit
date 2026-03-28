package issuemath

import "math"

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

func isZeroVector(values []float64) bool {
	for _, value := range values {
		if value != 0 {
			return false
		}
	}
	return true
}

func dotProduct(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

func roundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}
