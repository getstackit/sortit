package issuemath

import (
	"math"

	"gonum.org/v1/gonum/floats"
)

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
