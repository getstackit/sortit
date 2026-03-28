package issuemap

import "math"

func unitVec(v []float64) []float64 {
	out := make([]float64, len(v))
	copy(out, v)
	var mag float64
	for _, val := range out {
		mag += val * val
	}
	if mag > 0 {
		mag = math.Sqrt(mag)
		for i := range out {
			out[i] /= mag
		}
	}
	return out
}
