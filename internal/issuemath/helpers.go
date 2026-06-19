package issuemath

import (
	"math"

	"gonum.org/v1/gonum/floats"
)

func normalizeVector(vector []float64) {
	// floats.Dot(v, v) is the sum of squares through the unrolled kernel.
	magnitude := floats.Dot(vector, vector)
	if magnitude == 0 {
		return
	}
	floats.Scale(1/math.Sqrt(magnitude), vector)
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
	return floats.Dot(a, b)
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

func roundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}
