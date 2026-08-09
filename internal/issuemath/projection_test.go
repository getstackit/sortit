package issuemath

import (
	"fmt"
	"math"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/vectors"
)

func TestComputePositionsSingleIssueFallsBackToCenter(t *testing.T) {
	_, err := ComputePositions([]issues.Issue{
		{
			ID: "1",
			TagScores: []domain.TagRelevance{
				{Tag: "bug", Relevance: 1},
			},
		},
	}, []string{"bug"}, nil)
	if err == nil {
		t.Fatal("expected insufficient-data error for single-issue projection")
	}
}

func TestComputePositionsSingleTagReturnsError(t *testing.T) {
	items := []issues.Issue{
		{ID: "1", TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.9}}},
		{ID: "2", TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.7}}},
		{ID: "3", TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.5}}},
		{ID: "4", TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.4}}},
		{ID: "5", TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.3}}},
	}

	_, err := ComputePositions(items, []string{"bug"}, nil)
	if err == nil {
		t.Fatal("expected insufficient-dimensions error for single-tag projection")
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
		{ID: "a", Raw: "A well-described feature request about improving search ranking quality.", TagScores: []domain.TagRelevance{{Tag: "search", Relevance: 0.9}, {Tag: "backend", Relevance: 0.7}}, LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: mat(0.9)}},
		{ID: "b", Raw: "Fix the bug in the login page that causes a crash.", TagScores: []domain.TagRelevance{{Tag: "bug", Relevance: 0.8}, {Tag: "frontend", Relevance: 0.6}}, LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: mat(0.5)}},
		{ID: "c", Raw: "short", TagScores: []domain.TagRelevance{{Tag: "search", Relevance: 0.5}, {Tag: "bug", Relevance: 0.3}}, LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: mat(0.1)}},
		{ID: "d", Raw: "Add export to CSV for the dashboard metrics page.", TagScores: []domain.TagRelevance{{Tag: "frontend", Relevance: 0.7}, {Tag: "backend", Relevance: 0.5}}, LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: mat(0.6)}},
		{ID: "e", Raw: "Search results should highlight matching keywords in the snippet.", TagScores: []domain.TagRelevance{{Tag: "search", Relevance: 0.8}, {Tag: "frontend", Relevance: 0.4}}, LifecycleMetrics: &issues.IssueLifecycleMetrics{Maturity: mat(0.7)}},
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

// stabilityFixtureTags and stabilityFixture build a deterministic corpus
// with three well-separated tag clusters, so the top two eigenvalues are
// clearly separated and layout differences come from the pipeline rather
// than near-degenerate PCA.
var stabilityFixtureTags = []string{"search", "backend", "bug", "frontend", "perf", "infra"}

func stabilityFixture() []issues.Issue {
	clusters := [][2]string{
		{"search", "backend"},
		{"bug", "frontend"},
		{"perf", "infra"},
	}
	items := make([]issues.Issue, 0, 12)
	for i := range 12 {
		pair := clusters[i%3]
		jitter := float64(i) * 0.02
		items = append(items, issues.Issue{
			ID:  fmt.Sprintf("issue-%d", i),
			Raw: fmt.Sprintf("A reasonably detailed description of issue number %d touching %s and %s.", i, pair[0], pair[1]),
			TagScores: []domain.TagRelevance{
				{Tag: pair[0], Relevance: 0.9 - jitter},
				{Tag: pair[1], Relevance: 0.6 + jitter},
			},
		})
	}
	return items
}

func TestComputePositionsDeterministic(t *testing.T) {
	items := stabilityFixture()
	first, err := ComputePositions(items, stabilityFixtureTags, nil)
	if err != nil {
		t.Fatalf("ComputePositions: %v", err)
	}
	second, err := ComputePositions(items, stabilityFixtureTags, nil)
	if err != nil {
		t.Fatalf("ComputePositions: %v", err)
	}
	for id, a := range first {
		b := second[id]
		if a != b {
			t.Fatalf("same input produced different positions for %s: %+v vs %+v", id, a, b)
		}
	}
}

func TestComputePositionsOrderInvariant(t *testing.T) {
	items := stabilityFixture()
	reversed := make([]issues.Issue, len(items))
	for i, item := range items {
		reversed[len(items)-1-i] = item
	}

	forward, err := ComputePositions(items, stabilityFixtureTags, nil)
	if err != nil {
		t.Fatalf("ComputePositions: %v", err)
	}
	backward, err := ComputePositions(reversed, stabilityFixtureTags, nil)
	if err != nil {
		t.Fatalf("ComputePositions: %v", err)
	}

	const tolerance = 1e-6 // floating-point summation order differs
	for id, a := range forward {
		b := backward[id]
		if math.Abs(a.X-b.X) > tolerance || math.Abs(a.Y-b.Y) > tolerance {
			t.Errorf("reversed input moved %s: %+v vs %+v", id, a, b)
		}
	}
}

func TestComputePositionsAlignedAppendedIssueBarelyMovesExisting(t *testing.T) {
	items := stabilityFixture()
	previous, err := ComputePositions(items, stabilityFixtureTags, nil)
	if err != nil {
		t.Fatalf("ComputePositions: %v", err)
	}

	extended := append(append([]issues.Issue(nil), items...), issues.Issue{
		ID:  "issue-new",
		Raw: "A freshly filed issue about backend search latency regressions.",
		TagScores: []domain.TagRelevance{
			{Tag: "search", Relevance: 0.8},
			{Tag: "perf", Relevance: 0.5},
		},
	})

	aligned, err := ComputePositionsAligned(extended, stabilityFixtureTags, nil, previous)
	if err != nil {
		t.Fatalf("ComputePositionsAligned: %v", err)
	}

	const epsilon = 0.05
	for _, item := range items {
		before, after := previous[item.ID], aligned[item.ID]
		if math.Abs(before.X-after.X) > epsilon || math.Abs(before.Y-after.Y) > epsilon {
			t.Errorf("appending one issue moved %s by more than %v: %+v -> %+v", item.ID, epsilon, before, after)
		}
	}
}

func TestComputePositionsAlignedRecoversReflectedLayout(t *testing.T) {
	// If the previous layout is a mirror image of what PCA would produce,
	// the orthogonal Procrustes step must pick the reflection and land
	// issues back on their previous positions.
	items := stabilityFixture()
	base, err := ComputePositions(items, stabilityFixtureTags, nil)
	if err != nil {
		t.Fatalf("ComputePositions: %v", err)
	}

	mirrored := make(map[string]Position, len(base))
	for id, p := range base {
		mirrored[id] = Position{X: 1 - p.X, Y: p.Y}
	}

	aligned, err := ComputePositionsAligned(items, stabilityFixtureTags, nil, mirrored)
	if err != nil {
		t.Fatalf("ComputePositionsAligned: %v", err)
	}

	const epsilon = 0.02
	for id, want := range mirrored {
		got := aligned[id]
		if math.Abs(got.X-want.X) > epsilon || math.Abs(got.Y-want.Y) > epsilon {
			t.Errorf("alignment failed to recover mirrored layout for %s: want %+v, got %+v", id, want, got)
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
