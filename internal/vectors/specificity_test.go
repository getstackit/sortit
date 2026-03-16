package vectors

import "testing"

func TestNeighborhoodSpecificityEmpty(t *testing.T) {
	results := NeighborhoodSpecificity(nil)
	if results != nil {
		t.Fatalf("NeighborhoodSpecificity(nil) = %#v, want nil", results)
	}
}

func TestNeighborhoodSpecificityDeterministic(t *testing.T) {
	embeddings := [][]float64{
		{1, 0, 0},
		{0.98, 0.02, 0},
		{0.97, 0.03, 0},
		{0, 1, 0},
	}

	first := NeighborhoodSpecificity(embeddings)
	second := NeighborhoodSpecificity(embeddings)
	if len(first) != len(second) {
		t.Fatalf("result length mismatch: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("result[%d] mismatch: %#v vs %#v", i, first[i], second[i])
		}
	}
}

func TestNeighborhoodSpecificityDenseVsSparse(t *testing.T) {
	embeddings := [][]float64{
		{1, 0, 0},
		{0.99, 0.01, 0},
		{0.98, 0.02, 0},
		{0, 1, 0},
	}

	results := NeighborhoodSpecificity(embeddings)
	if len(results) != len(embeddings) {
		t.Fatalf("expected %d results, got %d", len(embeddings), len(results))
	}

	sparse := results[3].Specificity
	for i := 0; i < 3; i++ {
		if sparse <= results[i].Specificity {
			t.Fatalf("sparse point specificity %v should exceed dense point[%d] specificity %v", sparse, i, results[i].Specificity)
		}
	}
}

func TestNeighborhoodSpecificityIdenticalPointsAreNeutral(t *testing.T) {
	results := NeighborhoodSpecificity([][]float64{
		{1, 0, 0},
		{1, 0, 0},
		{1, 0, 0},
		{1, 0, 0},
	})

	for i, result := range results {
		if result.MeanNeighborDistance != 0 {
			t.Fatalf("result[%d].MeanNeighborDistance = %v, want 0", i, result.MeanNeighborDistance)
		}
		if result.Specificity != 0.5 {
			t.Fatalf("result[%d].Specificity = %v, want 0.5", i, result.Specificity)
		}
	}
}
