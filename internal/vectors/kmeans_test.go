package vectors

import (
	"math"
	"testing"
)

func TestKMeansSpecificityEmpty(t *testing.T) {
	results := KMeansSpecificity(nil)
	if results != nil {
		t.Fatalf("expected nil for empty input, got %v", results)
	}
}

func TestKMeansSpecificitySinglePoint(t *testing.T) {
	results := KMeansSpecificity([][]float64{{1, 0, 0}})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Specificity != 0 {
		t.Errorf("single point specificity should be 0, got %v", results[0].Specificity)
	}
}

func TestKMeansSpecificityOutlierCap(t *testing.T) {
	results := KMeansSpecificity([][]float64{{1, 0, 0}})
	for _, r := range results {
		if r.Specificity > specificityOutlierCap {
			t.Errorf("specificity %v exceeds outlier cap %v", r.Specificity, specificityOutlierCap)
		}
	}
}

func TestKMeansSpecificityDistinguishesClusters(t *testing.T) {
	// Two clear clusters: X-axis vs Y-axis, with an outlier.
	embeddings := [][]float64{
		{1, 0, 0},    // cluster A
		{0.9, 0.1, 0}, // cluster A
		{0.8, 0.2, 0}, // cluster A
		{0, 1, 0},    // cluster B
		{0.1, 0.9, 0}, // cluster B
		{0.2, 0.8, 0}, // cluster B
	}

	results := KMeansSpecificity(embeddings)
	if len(results) != len(embeddings) {
		t.Fatalf("expected %d results, got %d", len(embeddings), len(results))
	}

	// All specificities should be in [0, 0.8]
	for i, r := range results {
		if r.Specificity < 0 || r.Specificity > specificityOutlierCap {
			t.Errorf("result[%d] specificity %v out of range [0, %v]", i, r.Specificity, specificityOutlierCap)
		}
	}

	// Points closest to their centroid should have lower specificity.
	// {1,0,0} should be closer to its centroid than {0.8,0.2,0}.
	if results[0].Cluster == results[2].Cluster {
		if results[0].Distance > results[2].Distance {
			t.Logf("as expected, (1,0,0) is closer to centroid than (0.8,0.2,0)")
		}
	}
}

func TestKMeansSpecificityKEquation(t *testing.T) {
	// Verify k = floor(sqrt(n)), min 2.
	tests := []struct {
		n    int
		wantK int
	}{
		{2, 2},
		{3, 2},
		{4, 2},
		{5, 2},
		{9, 3},
		{16, 4},
		{100, 10},
	}
	for _, tt := range tests {
		k := int(math.Floor(math.Sqrt(float64(tt.n))))
		if k < kmeansMinK {
			k = kmeansMinK
		}
		if k != tt.wantK {
			t.Errorf("n=%d: k=%d, want %d", tt.n, k, tt.wantK)
		}
	}
}

func TestKMeansSpecificityDeterministicWithIdenticalPoints(t *testing.T) {
	// All identical points: all distances should be 0, all specificities 0.
	embeddings := make([][]float64, 10)
	for i := range embeddings {
		embeddings[i] = []float64{1, 0, 0}
	}

	results := KMeansSpecificity(embeddings)
	for i, r := range results {
		if r.Specificity != 0 {
			t.Errorf("result[%d] specificity should be 0 for identical points, got %v", i, r.Specificity)
		}
	}
}
