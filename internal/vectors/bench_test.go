package vectors

import (
	"math"
	"testing"
)

// benchDim mirrors a typical embedding dimension so the benchmarks exercise
// the same SIMD-friendly vector lengths the search path sees in production.
const benchDim = 1536

func benchVec(seed int) []float64 {
	v := make([]float64, benchDim)
	x := float64(seed)*0.6180339887 + 0.1
	for i := range v {
		x = math.Mod(x*1.3+float64(i)*0.0007, 2)
		v[i] = x - 1
	}
	return v
}

func BenchmarkCosineSimilarity(b *testing.B) {
	x, y := benchVec(1), benchVec(2)
	b.ReportAllocs()
	for b.Loop() {
		CosineSimilarity(x, y)
	}
}

func BenchmarkUnitCosineSimilarity(b *testing.B) {
	x, y := benchVec(1), benchVec(2)
	b.ReportAllocs()
	for b.Loop() {
		UnitCosineSimilarity(x, y)
	}
}

func BenchmarkMean(b *testing.B) {
	const n = 200
	vecs := make([][]float64, n)
	for i := range vecs {
		vecs[i] = benchVec(i)
	}
	b.ReportAllocs()
	for b.Loop() {
		Mean(vecs)
	}
}

func BenchmarkCenterUnit(b *testing.B) {
	v, mean := benchVec(1), benchVec(99)
	b.ReportAllocs()
	for b.Loop() {
		CenterUnit(v, mean)
	}
}
