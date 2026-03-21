package issuemap

import (
	"math"
	"testing"

	"splat/internal/issues"
	"splat/internal/vectors"
)

func TestComputePositionsSingleIssueFallsBackToCenter(t *testing.T) {
	positions, err := ComputePositions([]issues.Issue{
		{
			ID: "1",
			TagScores: []TagRelevance{
				{Tag: "bug", Relevance: 1},
			},
		},
	}, []string{"bug"}, nil)
	if err != nil {
		t.Fatalf("ComputePositions returned error: %v", err)
	}

	pos, ok := positions["1"]
	if !ok {
		t.Fatal("expected position for issue 1")
	}
	if pos.X != 0.5 || pos.Y != 0.5 {
		t.Fatalf("expected centered fallback position, got %+v", pos)
	}
}

func TestComputePositionsSingleTagUsesFallbackLayout(t *testing.T) {
	items := []issues.Issue{
		{ID: "1", TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.9}}},
		{ID: "2", TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.7}}},
		{ID: "3", TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.5}}},
	}

	positions, err := ComputePositions(items, []string{"bug"}, nil)
	if err != nil {
		t.Fatalf("ComputePositions returned error: %v", err)
	}

	if len(positions) != len(items) {
		t.Fatalf("expected %d positions, got %d", len(items), len(positions))
	}

	seen := map[Position]struct{}{}
	for _, issue := range items {
		pos, ok := positions[issue.ID]
		if !ok {
			t.Fatalf("missing position for issue %s", issue.ID)
		}
		if pos.X < 0 || pos.X > 1 || pos.Y < 0 || pos.Y > 1 {
			t.Fatalf("fallback position out of range for issue %s: %+v", issue.ID, pos)
		}
		seen[pos] = struct{}{}
	}

	if len(seen) != len(items) {
		t.Fatalf("expected distinct fallback positions, got %d unique positions", len(seen))
	}
}

func TestIssueProjectionWeights(t *testing.T) {
	mat := func(v float64) *float64 { return &v }

	items := []issues.Issue{
		{ID: "rich", Raw: "This is a detailed issue with plenty of structured content describing the problem, proposed solution, and acceptance criteria for the implementation.", LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: mat(0.8)}},
		{ID: "sparse", Raw: "fix bug", LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: mat(0.2)}},
		{ID: "no-metrics", Raw: "A moderately detailed issue without lifecycle metrics populated yet."},
	}

	weights := issueProjectionWeights(items)

	if len(weights) != 3 {
		t.Fatalf("expected 3 weights, got %d", len(weights))
	}

	// Rich + mature issue should have higher weight than sparse + immature
	if weights[0] <= weights[1] {
		t.Errorf("expected rich issue weight (%v) > sparse issue weight (%v)", weights[0], weights[1])
	}

	// All weights should be >= floor
	for i, w := range weights {
		if w < 0.1 {
			t.Errorf("weight[%d] = %v, below floor 0.1", i, w)
		}
	}
}

func TestWeightedPCAProducesValidPositions(t *testing.T) {
	mat := func(v float64) *float64 { return &v }

	items := []issues.Issue{
		{ID: "a", Raw: "A well-described feature request about improving search ranking quality.", TagScores: []TagRelevance{{Tag: "search", Relevance: 0.9}, {Tag: "backend", Relevance: 0.7}}, LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: mat(0.9)}},
		{ID: "b", Raw: "Fix the bug in the login page that causes a crash.", TagScores: []TagRelevance{{Tag: "bug", Relevance: 0.8}, {Tag: "frontend", Relevance: 0.6}}, LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: mat(0.5)}},
		{ID: "c", Raw: "short", TagScores: []TagRelevance{{Tag: "search", Relevance: 0.5}, {Tag: "bug", Relevance: 0.3}}, LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: mat(0.1)}},
	}

	positions, err := ComputePositions(items, []string{"search", "backend", "bug", "frontend"}, nil)
	if err != nil {
		t.Fatalf("ComputePositions returned error: %v", err)
	}

	for _, item := range items {
		pos, ok := positions[item.ID]
		if !ok {
			t.Fatalf("missing position for %s", item.ID)
		}
		if pos.X < 0 || pos.X > 1 || pos.Y < 0 || pos.Y > 1 {
			t.Fatalf("position out of range for %s: %+v", item.ID, pos)
		}
	}
}

func TestNormalizeRobustClipsOutliersBeforeScaling(t *testing.T) {
	vals := []float64{0, 1, 2, 3, 100}

	normalizeRobust(vals, 0.05, 0.95)

	if vals[0] != 0.05 {
		t.Fatalf("expected lower bound to map to 0.05, got %v", vals[0])
	}
	if vals[3] >= 0.8 {
		t.Fatalf("expected inlier values to avoid being crushed by outlier, got %v", vals[3])
	}
	if vals[4] != 0.95 {
		t.Fatalf("expected clipped outlier to map to 0.95, got %v", vals[4])
	}
}

func TestBuildTagCovarianceShrinkage(t *testing.T) {
	// Non-orthogonal embeddings so off-diagonal entries are nonzero.
	tagEmb := map[string][]float64{
		"a": unitVec([]float64{1, 0.5, 0, 0}),
		"b": unitVec([]float64{0.5, 1, 0, 0}),
		"c": unitVec([]float64{0, 0, 1, 0}),
	}
	tags := []string{"a", "b", "c"}

	cov := buildTagCovariance(tags, tagEmb)

	// Diagonal should remain 1.0 (self-similarity unchanged by shrinkage).
	for i := range tags {
		if v := cov.At(i, i); math.Abs(v-1.0) > 1e-9 {
			t.Errorf("diagonal (%d,%d) = %f, want 1.0", i, i, v)
		}
	}

	// Off-diagonal (a,b) should be reduced relative to raw cosine similarity.
	rawAB := vectors.UnitCosineSimilarity(tagEmb["a"], tagEmb["b"])
	shrunkAB := cov.At(0, 1)
	if shrunkAB >= rawAB-1e-9 {
		t.Errorf("expected shrunk off-diagonal (%f) < raw (%f)", shrunkAB, rawAB)
	}
	if shrunkAB <= 0 {
		t.Errorf("expected positive off-diagonal for correlated tags, got %f", shrunkAB)
	}

	// Matrix should be symmetric.
	if math.Abs(cov.At(0, 1)-cov.At(1, 0)) > 1e-12 {
		t.Errorf("matrix not symmetric: (0,1)=%f, (1,0)=%f", cov.At(0, 1), cov.At(1, 0))
	}
}

func TestBuildTagCovarianceNoEmbeddingsUnchanged(t *testing.T) {
	// Without embeddings, result should be identity (shrinkage is no-op).
	cov := buildTagCovariance([]string{"a", "b", "c"}, nil)
	for i := range 3 {
		for j := range 3 {
			expected := 0.0
			if i == j {
				expected = 1.0
			}
			if math.Abs(cov.At(i, j)-expected) > 1e-12 {
				t.Errorf("(%d,%d) = %f, want %f", i, j, cov.At(i, j), expected)
			}
		}
	}
}

func TestCorrelationShrinkageAlpha(t *testing.T) {
	// Identity matrix → α should be 1.0 (no shrinkage needed).
	identity := []float64{
		1, 0, 0,
		0, 1, 0,
		0, 0, 1,
	}
	alpha := correlationShrinkageAlpha(identity, 3)
	if math.Abs(alpha-1.0) > 1e-9 {
		t.Errorf("identity matrix: expected α=1.0, got %f", alpha)
	}

	// Highly correlated matrix → α should be low.
	correlated := []float64{
		1, 0.9, 0.9,
		0.9, 1, 0.9,
		0.9, 0.9, 1,
	}
	alpha = correlationShrinkageAlpha(correlated, 3)
	if alpha > 0.25 {
		t.Errorf("highly correlated matrix: expected low α, got %f", alpha)
	}
	if alpha < 0.1 {
		t.Errorf("α should be clamped to minimum 0.1, got %f", alpha)
	}

	// Moderately correlated → α somewhere in between.
	moderate := []float64{
		1, 0.3, 0.1,
		0.3, 1, 0.2,
		0.1, 0.2, 1,
	}
	alpha = correlationShrinkageAlpha(moderate, 3)
	if alpha < 0.9 || alpha > 1.0 {
		t.Errorf("moderately correlated matrix: expected α near 0.95, got %f", alpha)
	}
}
