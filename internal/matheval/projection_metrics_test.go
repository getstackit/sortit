package matheval

import (
	"fmt"
	"math"
	"testing"

	"sortit/internal/issuemath"
)

// gridPositions lays n points on a 1-D line (x = 0..n-1, y = 0) so nearest
// neighbors by Euclidean distance are the index-adjacent points.
func gridPositions(n int) map[string]issuemath.Position {
	pos := make(map[string]issuemath.Position, n)
	for i := range n {
		pos[fmt.Sprintf("p%02d", i)] = issuemath.Position{X: float64(i), Y: 0}
	}
	return pos
}

func gridIDs(n int) []string {
	ids := make([]string, n)
	for i := range n {
		ids[i] = fmt.Sprintf("p%02d", i)
	}
	return ids
}

// TestMeanKNNOverlapIdentityIsOne: a layout whose reference neighborhood is
// itself scores a perfect 1.0 — the perfect-grid case.
func TestMeanKNNOverlapIdentityIsOne(t *testing.T) {
	ids := gridIDs(10)
	layout := layoutNeighbors(gridPositions(10), ids, 3)
	if got := meanKNNOverlap(ids, layout, layout, 3); math.Abs(got-1) > 1e-9 {
		t.Errorf("identity overlap = %.4f, want 1.0", got)
	}
}

// TestMeanKNNOverlapShuffledIsChance: a layout compared against an unrelated
// (shuffled) reference neighborhood scores near the chance level k/(n−1), far
// below 1.
func TestMeanKNNOverlapShuffledIsChance(t *testing.T) {
	n, k := 20, 5
	ids := gridIDs(n)
	layout := layoutNeighbors(gridPositions(n), ids, k)

	// Shuffled layout: reverse the coordinate assignment so each point's
	// neighborhood is unrelated to the original.
	shuffled := make(map[string]issuemath.Position, n)
	for i, id := range ids {
		shuffled[id] = issuemath.Position{X: float64((i * 7) % n), Y: float64((i * 13) % n)}
	}
	ref := layoutNeighbors(shuffled, ids, k)

	got := meanKNNOverlap(ids, layout, ref, k)
	chance := float64(k) / float64(n-1)
	if got > chance+0.25 {
		t.Errorf("shuffled overlap = %.4f, expected near chance (%.4f)", got, chance)
	}
}

// TestEmbeddingNeighborsGroupsByCosine: cosine-nearest neighbors of a vector
// are the other members of its axis-aligned group.
func TestEmbeddingNeighborsGroupsByCosine(t *testing.T) {
	emb := map[string][]float64{
		"a1": {1, 0, 0}, "a2": {0.9, 0.1, 0}, "a3": {0.8, 0.2, 0},
		"b1": {0, 1, 0}, "b2": {0, 0.9, 0.1}, "b3": {0, 0.8, 0.2},
	}
	ids := []string{"a1", "a2", "a3", "b1", "b2", "b3"}
	nn := embeddingNeighbors(emb, ids, 2)
	for _, id := range []string{"a1", "a2", "a3"} {
		for _, got := range nn[id] {
			if got[0] != 'a' {
				t.Errorf("%s neighbor %s is not in group a", id, got)
			}
		}
	}
}

// TestMeanDomainPurityPerfectAndInterleaved: a layout that clusters each domain
// scores near 1; interleaving the domains drops purity sharply.
func TestMeanDomainPurityPerfectAndInterleaved(t *testing.T) {
	domainOf := map[string]string{
		"x0": "x", "x1": "x", "x2": "x", "x3": "x",
		"y0": "y", "y1": "y", "y2": "y", "y3": "y",
	}
	ids := []string{"x0", "x1", "x2", "x3", "y0", "y1", "y2", "y3"}

	// Clustered: x domain near origin, y domain far away.
	clustered := map[string]issuemath.Position{
		"x0": {X: 0, Y: 0}, "x1": {X: 0.1, Y: 0}, "x2": {X: 0, Y: 0.1}, "x3": {X: 0.1, Y: 0.1},
		"y0": {X: 10, Y: 10}, "y1": {X: 10.1, Y: 10}, "y2": {X: 10, Y: 10.1}, "y3": {X: 10.1, Y: 10.1},
	}
	pure := meanDomainPurity(ids, layoutNeighbors(clustered, ids, 3), domainOf)
	if pure < 0.99 {
		t.Errorf("clustered domain purity = %.4f, want ≈1", pure)
	}

	// Interleaved: alternate the two domains along a line.
	interleaved := map[string]issuemath.Position{
		"x0": {X: 0, Y: 0}, "y0": {X: 1, Y: 0}, "x1": {X: 2, Y: 0}, "y1": {X: 3, Y: 0},
		"x2": {X: 4, Y: 0}, "y2": {X: 5, Y: 0}, "x3": {X: 6, Y: 0}, "y3": {X: 7, Y: 0},
	}
	mixed := meanDomainPurity(ids, layoutNeighbors(interleaved, ids, 3), domainOf)
	if mixed >= pure {
		t.Errorf("interleaved purity %.4f should be below clustered %.4f", mixed, pure)
	}
}

// TestSilhouetteSeparatedIsPositive: two well-separated ground-truth regions
// yield a high positive silhouette.
func TestSilhouetteSeparatedIsPositive(t *testing.T) {
	labels := map[string]string{"a0": "a", "a1": "a", "a2": "a", "b0": "b", "b1": "b", "b2": "b"}
	pos := map[string]issuemath.Position{
		"a0": {X: 0, Y: 0}, "a1": {X: 0.1, Y: 0.1}, "a2": {X: 0, Y: 0.1},
		"b0": {X: 5, Y: 5}, "b1": {X: 5.1, Y: 5}, "b2": {X: 5, Y: 5.1},
	}
	if got := silhouetteByLabel(pos, labels); got < 0.5 {
		t.Errorf("separated silhouette = %.4f, want > 0.5", got)
	}
}

// TestSilhouetteCollapsedIsNonPositive: a collapsed layout (all points
// coincident) has no cluster structure, so the silhouette is ≤ 0.
func TestSilhouetteCollapsedIsNonPositive(t *testing.T) {
	labels := map[string]string{"a0": "a", "a1": "a", "b0": "b", "b1": "b"}
	pos := map[string]issuemath.Position{
		"a0": {X: 1, Y: 1}, "a1": {X: 1, Y: 1}, "b0": {X: 1, Y: 1}, "b1": {X: 1, Y: 1},
	}
	if got := silhouetteByLabel(pos, labels); got > 1e-9 {
		t.Errorf("collapsed silhouette = %.4f, want ≤ 0", got)
	}
}

// TestSilhouetteInterleavedIsNegative: regions whose members sit closer to the
// other region than their own score a negative silhouette.
func TestSilhouetteInterleavedIsNegative(t *testing.T) {
	labels := map[string]string{"a0": "a", "a1": "a", "b0": "b", "b1": "b"}
	// a and b alternate, so each point's nearest neighbor is the other label.
	pos := map[string]issuemath.Position{
		"a0": {X: 0, Y: 0}, "b0": {X: 1, Y: 0}, "a1": {X: 2, Y: 0}, "b1": {X: 3, Y: 0},
	}
	if got := silhouetteByLabel(pos, labels); got >= 0 {
		t.Errorf("interleaved silhouette = %.4f, want < 0", got)
	}
}

// TestLayoutDisplacementsZeroWhenUnchanged: an identical refresh moves nothing —
// the zero-mutation stability case.
func TestLayoutDisplacementsZeroWhenUnchanged(t *testing.T) {
	pos := gridPositions(6)
	disp := layoutDisplacements(pos, pos)
	if len(disp) != 6 {
		t.Fatalf("expected 6 displacements, got %d", len(disp))
	}
	mean, p95 := meanAndP95(disp)
	if mean != 0 || p95 != 0 {
		t.Errorf("unchanged layout displacement mean=%.6f p95=%.6f, want 0/0", mean, p95)
	}
}

// TestLayoutDisplacementsOnlySharedIssues: issues absent from one layout carry
// no displacement (added/removed by a mutation), and a uniform shift is measured
// exactly.
func TestLayoutDisplacementsOnlySharedIssues(t *testing.T) {
	prev := map[string]issuemath.Position{"a": {X: 0, Y: 0}, "b": {X: 1, Y: 0}, "gone": {X: 9, Y: 9}}
	cur := map[string]issuemath.Position{"a": {X: 0, Y: 0}, "b": {X: 1, Y: 3}, "new": {X: 5, Y: 5}}
	disp := layoutDisplacements(prev, cur)
	if len(disp) != 2 {
		t.Fatalf("expected 2 shared displacements, got %d", len(disp))
	}
	mean, _ := meanAndP95(disp)
	// a moved 0, b moved 3 → mean 1.5.
	if math.Abs(mean-1.5) > 1e-9 {
		t.Errorf("mean displacement = %.4f, want 1.5", mean)
	}
}
