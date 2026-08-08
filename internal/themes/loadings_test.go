package themes

import (
	"reflect"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issuemath"
)

func TestAnchoredAny(t *testing.T) {
	tagIndex := map[string]int{"alpha": 0, "beta": 1}

	cases := []struct {
		name   string
		scores []domain.TagRelevance
		want   bool
	}{
		{"no scores", nil, false},
		{"unknown tag only", []domain.TagRelevance{{Tag: "gamma", Relevance: 0.9}}, false},
		{"one known tag", []domain.TagRelevance{{Tag: "alpha", Relevance: 0.5}}, true},
		{"zero-relevance still anchored", []domain.TagRelevance{{Tag: "beta", Relevance: 0}}, true},
		{"mixed known+unknown", []domain.TagRelevance{{Tag: "gamma"}, {Tag: "alpha"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := anchoredAny(tc.scores, tagIndex); got != tc.want {
				t.Fatalf("anchoredAny = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRawLoading asserts raw f = Loading·LoadingNorm is reconstructed at full
// width, and that a residual-only vector (nil Loading / zero norm) or a shape
// mismatch yields an all-zero row.
func TestRawLoading(t *testing.T) {
	got := rawLoading(issuemath.RidgeVectors{
		Loading:     []float64{0.6, -0.8},
		LoadingNorm: 5,
	}, 2)
	want := []float64{3, -4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rawLoading = %v, want %v", got, want)
	}

	zero := make([]float64, 3)
	if got := rawLoading(issuemath.RidgeVectors{Loading: nil, LoadingNorm: 0}, 3); !reflect.DeepEqual(got, zero) {
		t.Fatalf("residual-only rawLoading = %v, want %v", got, zero)
	}
	if got := rawLoading(issuemath.RidgeVectors{Loading: []float64{1}, LoadingNorm: 2}, 3); !reflect.DeepEqual(got, zero) {
		t.Fatalf("shape-mismatch rawLoading = %v, want %v", got, zero)
	}
}
