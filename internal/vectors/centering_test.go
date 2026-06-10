package vectors

import (
	"math"
	"testing"
)

func TestMean(t *testing.T) {
	t.Run("empty input returns nil", func(t *testing.T) {
		if got := Mean(nil); got != nil {
			t.Fatalf("Mean(nil) = %v, want nil", got)
		}
		if got := Mean([][]float64{}); got != nil {
			t.Fatalf("Mean(empty) = %v, want nil", got)
		}
		if got := Mean([][]float64{nil, {}}); got != nil {
			t.Fatalf("Mean(all empty) = %v, want nil", got)
		}
	})

	t.Run("single vector is its own mean", func(t *testing.T) {
		got := Mean([][]float64{{1, 2, 3}})
		want := []float64{1, 2, 3}
		assertVectorEquals(t, got, want)
	})

	t.Run("component-wise mean", func(t *testing.T) {
		got := Mean([][]float64{{1, 0}, {0, 1}, {2, 2}})
		want := []float64{1, 1}
		assertVectorEquals(t, got, want)
	})

	t.Run("skips dimension mismatches", func(t *testing.T) {
		got := Mean([][]float64{{2, 4}, {1, 2, 3}, {0, 0}})
		want := []float64{1, 2}
		assertVectorEquals(t, got, want)
	})
}

func TestCenterUnit(t *testing.T) {
	t.Run("empty vector returns nil", func(t *testing.T) {
		if got := CenterUnit(nil, []float64{1}); got != nil {
			t.Fatalf("CenterUnit(nil) = %v, want nil", got)
		}
	})

	t.Run("centers and renormalizes", func(t *testing.T) {
		got := CenterUnit([]float64{1, 1}, []float64{1, 0})
		want := []float64{0, 1}
		assertVectorEquals(t, got, want)
		assertUnitLength(t, got)
	})

	t.Run("does not mutate input", func(t *testing.T) {
		v := []float64{1, 1}
		_ = CenterUnit(v, []float64{1, 0})
		assertVectorEquals(t, v, []float64{1, 1})
	})

	t.Run("nil mean falls back to normalized input", func(t *testing.T) {
		got := CenterUnit([]float64{3, 4}, nil)
		want := []float64{0.6, 0.8}
		assertVectorEquals(t, got, want)
	})

	t.Run("dimension mismatch falls back to normalized input", func(t *testing.T) {
		got := CenterUnit([]float64{3, 4}, []float64{1, 2, 3})
		want := []float64{0.6, 0.8}
		assertVectorEquals(t, got, want)
	})

	t.Run("zero after centering falls back to uncentered", func(t *testing.T) {
		// Single-vector corpus: the mean equals the vector itself.
		got := CenterUnit([]float64{3, 4}, []float64{3, 4})
		want := []float64{0.6, 0.8}
		assertVectorEquals(t, got, want)
	})

	t.Run("zero input stays zero", func(t *testing.T) {
		got := CenterUnit([]float64{0, 0}, []float64{1, 2})
		assertVectorEquals(t, got, []float64{0, 0})
	})
}

func assertVectorEquals(t *testing.T, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func assertUnitLength(t *testing.T, v []float64) {
	t.Helper()
	var mag float64
	for _, x := range v {
		mag += x * x
	}
	if math.Abs(math.Sqrt(mag)-1) > 1e-12 {
		t.Fatalf("vector %v is not unit length", v)
	}
}
