package regions

import (
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

const authTag = "auth"

func TestComputeMassEmptyCorpus(t *testing.T) {
	mass, open, closed := ComputeMass(nil, domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag})
	if mass != 0 || open != 0 || closed != 0 {
		t.Fatalf("expected zeros, got mass=%d open=%d closed=%d", mass, open, closed)
	}
}

func TestComputeMassFloorEdge(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: MembershipFloor - 0.001}}},
		{ID: "b", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: MembershipFloor}}},
		{ID: "c", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: MembershipFloor + 0.01}}},
	}
	mass, open, closed := ComputeMass(items, auth)
	if mass != 2 {
		t.Fatalf("mass = %d, want 2", mass)
	}
	if open != 2 {
		t.Fatalf("open = %d, want 2", open)
	}
	if closed != 0 {
		t.Fatalf("closed = %d, want 0", closed)
	}
}

func TestComputeMassStatusSplit(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.8}}},
		{ID: "b", Status: issues.StatusClosed, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.6}}},
		{ID: "c", Status: issues.StatusClosed, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.5}}},
	}
	mass, open, closed := ComputeMass(items, auth)
	if mass != 3 || open != 1 || closed != 2 {
		t.Fatalf("mass=%d open=%d closed=%d, want 3/1/2", mass, open, closed)
	}
}

func TestComputeMassExcludesNegationOnly(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0, Negation: new(0.6)}}},
	}
	mass, _, _ := ComputeMass(items, auth)
	if mass != 0 {
		t.Fatalf("mass = %d, want 0 (negation-only excluded)", mass)
	}
}

func TestComputeAgeBucketsBoundaries(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}
	mkIssue := func(id string, ageDays float64) issues.Issue {
		return issues.Issue{
			ID:        id,
			Status:    issues.StatusOpen,
			CreatedAt: now.Add(-time.Duration(ageDays * float64(24*time.Hour))),
			TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.7}},
		}
	}
	items := []issues.Issue{
		mkIssue("just-now", 0),
		mkIssue("under-1w", 6.9),
		mkIssue("exactly-1w", 7),
		mkIssue("inside-1-4w", 14),
		mkIssue("exactly-4w", 28),
		mkIssue("inside-1-3m", 60),
		mkIssue("exactly-3m", 90),
		mkIssue("over-3m", 365),
	}
	buckets := ComputeAgeBuckets(items, auth, now)
	if len(buckets) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(buckets))
	}
	want := map[string]int{
		AgeBucketUnderOneWeek:     2, // just-now, under-1w
		AgeBucketOneToFourWeeks:   2, // exactly-1w, inside-1-4w
		AgeBucketOneToThreeMonths: 2, // exactly-4w, inside-1-3m
		AgeBucketThreeMonthsPlus:  2, // exactly-3m, over-3m
	}
	for _, b := range buckets {
		if got := b.Count; got != want[b.Label] {
			t.Errorf("bucket %q = %d, want %d", b.Label, got, want[b.Label])
		}
	}
}

func TestComputeAgeBucketsExcludesClosed(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}
	items := []issues.Issue{
		{
			ID:        "a",
			Status:    issues.StatusClosed,
			CreatedAt: now.Add(-time.Hour),
			TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.9}},
		},
	}
	buckets := ComputeAgeBuckets(items, auth, now)
	for _, b := range buckets {
		if b.Count != 0 {
			t.Fatalf("closed issue should be excluded; bucket %q = %d", b.Label, b.Count)
		}
	}
}

func TestComputeMetricsZeroMassReturnsFalse(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window := domain.TimeWindow{Label: "30d", End: now}
	_, ok := ComputeMetrics(nil, nil, domain.RegionKey{Kind: domain.RegionKindTag, ID: authTag}, window, now)
	if ok {
		t.Fatal("expected ok=false for empty corpus")
	}
}

func TestListRegionsWithMetricsSortedByMass(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window := domain.TimeWindow{Label: "30d", End: now}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.9}}},
		{ID: "b", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.5}}},
		{ID: "c", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: "billing", Relevance: 0.7}}},
	}
	regions := ListRegionsWithMetrics(items, nil, window, now)
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(regions))
	}
	if regions[0].Region.Key.ID != authTag {
		t.Fatalf("expected auth first (higher mass); got %q", regions[0].Region.Key.ID)
	}
	if regions[0].Metrics.Mass != 2 || regions[1].Metrics.Mass != 1 {
		t.Fatalf("expected masses 2 and 1, got %d and %d", regions[0].Metrics.Mass, regions[1].Metrics.Mass)
	}
}

func TestComputeGrowthInWindow(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	mk := func(daysAgo float64, relevance float64) issues.Issue {
		return issues.Issue{
			Status:    issues.StatusOpen,
			CreatedAt: now.Add(-time.Duration(daysAgo * float64(24*time.Hour))),
			TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: relevance}},
		}
	}
	items := []issues.Issue{
		mk(2, 0.9),  // in 7d, 30d, 90d
		mk(10, 0.8), // in 30d, 90d
		mk(50, 0.7), // in 90d only
		mk(5, 0.1),  // never (below floor)
	}

	window7d, _ := ParseTimeWindow("7d", now)
	growth7d := ComputeGrowth(items, auth, window7d)
	if growth7d == nil || growth7d.Count != 1 {
		t.Fatalf("7d: expected count 1, got %+v", growth7d)
	}

	window30d, _ := ParseTimeWindow("30d", now)
	growth30d := ComputeGrowth(items, auth, window30d)
	if growth30d == nil || growth30d.Count != 2 {
		t.Fatalf("30d: expected count 2, got %+v", growth30d)
	}

	window90d, _ := ParseTimeWindow("90d", now)
	growth90d := ComputeGrowth(items, auth, window90d)
	if growth90d == nil || growth90d.Count != 3 {
		t.Fatalf("90d: expected count 3, got %+v", growth90d)
	}
}

func TestComputeGrowthReturnsNilForAllWindow(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	items := []issues.Issue{
		{Status: issues.StatusOpen, CreatedAt: now.Add(-time.Hour), TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}},
	}
	window, _ := ParseTimeWindow("all", now)
	if got := ComputeGrowth(items, auth, window); got != nil {
		t.Fatalf("expected nil for all window; got %+v", got)
	}
}

func TestComputeGrowthRespectsCurrentMembership(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	items := []issues.Issue{
		{
			Status:    issues.StatusOpen,
			CreatedAt: now.Add(-time.Hour),
			TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.1}},
		},
	}
	window, _ := ParseTimeWindow("30d", now)
	growth := ComputeGrowth(items, auth, window)
	if growth == nil || growth.Count != 0 {
		t.Fatalf("expected count 0 for issue below floor; got %+v", growth)
	}
}

func TestComputeGrowthPerDay(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	items := make([]issues.Issue, 0, 6)
	for i := range 6 {
		items = append(items, issues.Issue{
			Status:    issues.StatusOpen,
			CreatedAt: now.Add(-time.Duration(i+1) * 24 * time.Hour),
			TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.7}},
		})
	}
	window, _ := ParseTimeWindow("30d", now)
	growth := ComputeGrowth(items, auth, window)
	if growth == nil {
		t.Fatal("expected non-nil rate")
	}
	if growth.Count != 6 {
		t.Fatalf("count = %d, want 6", growth.Count)
	}
	if growth.PerDay != 0.2 {
		t.Fatalf("perDay = %v, want 0.20", growth.PerDay)
	}
}

func TestComputeClosureInWindow(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	mkClosed := func(daysAgo float64) issues.Issue {
		t := now.Add(-time.Duration(daysAgo * float64(24*time.Hour)))
		return issues.Issue{
			Status:    issues.StatusClosed,
			CreatedAt: t.Add(-30 * 24 * time.Hour),
			ClosedAt:  &t,
			TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.8}},
		}
	}
	items := []issues.Issue{
		mkClosed(2),
		mkClosed(15),
		mkClosed(60),
		{
			Status:    issues.StatusOpen,
			CreatedAt: now.Add(-24 * time.Hour),
			TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}},
		},
	}

	window30d, _ := ParseTimeWindow("30d", now)
	closure := ComputeClosure(items, auth, window30d)
	if closure == nil || closure.Count != 2 {
		t.Fatalf("30d: expected count 2 (2-day, 15-day); got %+v", closure)
	}

	window90d, _ := ParseTimeWindow("90d", now)
	closure90 := ComputeClosure(items, auth, window90d)
	if closure90 == nil || closure90.Count != 3 {
		t.Fatalf("90d: expected count 3; got %+v", closure90)
	}
}

func TestComputeClosureSkipsOpenStatus(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	closedAt := now.Add(-24 * time.Hour)
	items := []issues.Issue{
		{
			Status:    issues.StatusOpen,
			CreatedAt: now.Add(-30 * 24 * time.Hour),
			ClosedAt:  &closedAt, // paranoia: closedAt set but status open
			TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}},
		},
	}
	window, _ := ParseTimeWindow("30d", now)
	closure := ComputeClosure(items, auth, window)
	if closure == nil || closure.Count != 0 {
		t.Fatalf("expected count 0 when status is open; got %+v", closure)
	}
}

func TestComputeDensityNilForSmallSample(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	items := []issues.Issue{
		{Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: []float64{1, 0}},
		{Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: []float64{1, 0}},
	}
	if got := ComputeDensity(items, auth); got != nil {
		t.Fatalf("expected nil for < 5 members; got %+v", got)
	}
}

func TestComputeDensityIdenticalEmbeddingsApproachOne(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	items := make([]issues.Issue, 0, 5)
	for range 5 {
		items = append(items, issues.Issue{
			Status:    issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}},
			Embedding: []float64{1, 0, 0},
		})
	}
	density := ComputeDensity(items, auth)
	if density == nil {
		t.Fatal("expected non-nil density")
	}
	if *density < 0.99 {
		t.Fatalf("expected density ~1.0 for identical embeddings; got %v", *density)
	}
}

func TestComputeDensityOrthogonalEmbeddingsLow(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	dim := 5
	items := make([]issues.Issue, 0, dim)
	for i := range dim {
		vec := make([]float64, dim)
		vec[i] = 1
		items = append(items, issues.Issue{
			Status:    issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}},
			Embedding: vec,
		})
	}
	density := ComputeDensity(items, auth)
	if density == nil {
		t.Fatal("expected non-nil density")
	}
	// For 5 orthogonal unit vectors, centroid is (0.2,0.2,0.2,0.2,0.2) and
	// each vector has cosine sim 0.2/sqrt(0.2) ≈ 0.447 with the centroid.
	if *density > 0.5 {
		t.Fatalf("expected low density for orthogonal embeddings; got %v", *density)
	}
}

func TestComputeDensityCentersAgainstFullCorpus(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	members := []issues.Issue{
		{ID: "a1", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: []float64{10, 1, 0, 0}},
		{ID: "a2", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: []float64{10, -1, 0, 0}},
		{ID: "a3", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: []float64{10, 0, 1, 0}},
		{ID: "a4", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: []float64{10, 0, -1, 0}},
		{ID: "a5", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: []float64{10, 0.7, 0.7, 0}},
	}
	items := append([]issues.Issue(nil), members...)
	items = append(items,
		issues.Issue{ID: "b1", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "billing", Relevance: 0.9}}, Embedding: []float64{10, 0, 0, 1}},
		issues.Issue{ID: "b2", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "billing", Relevance: 0.9}}, Embedding: []float64{10, 0, 0, -1}},
		issues.Issue{ID: "b3", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "billing", Relevance: 0.9}}, Embedding: []float64{10, 0.4, 0, 1}},
		issues.Issue{ID: "b4", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "billing", Relevance: 0.9}}, Embedding: []float64{10, -0.4, 0, -1}},
		issues.Issue{ID: "b5", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "billing", Relevance: 0.9}}, Embedding: []float64{10, 0, 0.4, 1}},
	)

	rawDensity := ComputeDensityMembers(members)
	centeredDensity := ComputeDensity(items, auth)
	if rawDensity == nil || centeredDensity == nil {
		t.Fatalf("expected both densities, got raw=%+v centered=%+v", rawDensity, centeredDensity)
	}
	if *rawDensity < 0.95 {
		t.Fatalf("test setup expected raw common-direction density to be high, got %v", *rawDensity)
	}
	if *centeredDensity >= *rawDensity-0.2 {
		t.Fatalf("expected corpus centering to materially reduce density; raw=%v centered=%v", *rawDensity, *centeredDensity)
	}
}

func TestComputeDensitySkipsIssuesWithoutEmbeddings(t *testing.T) {
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	items := []issues.Issue{
		{Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: []float64{1, 0}},
		{Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: nil},
		{Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: nil},
		{Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}, Embedding: nil},
	}
	if got := ComputeDensity(items, auth); got != nil {
		t.Fatalf("expected nil when only 1 member has an embedding; got %+v", got)
	}
}

func TestComputeCorpusOrphansEmpty(t *testing.T) {
	got := ComputeCorpusOrphans(nil, nil)
	if got.Total != 0 || got.Open != 0 || got.Fraction != 0 {
		t.Fatalf("expected zeros for empty corpus, got %+v", got)
	}
}

func TestComputeCorpusOrphansIdentifiesUnalignedIssues(t *testing.T) {
	tags := []issues.Tag{
		{Name: "auth", Embedding: []float64{1, 0, 0}},
		{Name: "billing", Embedding: []float64{0, 1, 0}},
	}
	items := []issues.Issue{
		// In-region: top tag relevance >= floor.
		{ID: "1", Status: issues.StatusOpen, Embedding: []float64{1, 0, 0}, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.8}}},
		// Below floor but aligned with auth — not an orphan.
		{ID: "2", Status: issues.StatusOpen, Embedding: []float64{0.9, 0.1, 0}, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.2}}},
		// Below floor AND embedding orthogonal to all tag embeddings — orphan.
		{ID: "3", Status: issues.StatusOpen, Embedding: []float64{0, 0, 1}, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.1}}},
		// Below floor, orthogonal, closed — orphan (counted but not in Open).
		{ID: "4", Status: issues.StatusClosed, Embedding: []float64{0, 0, 1}, TagScores: []domain.TagRelevance{}},
	}
	got := ComputeCorpusOrphans(items, tags)
	if got.Total != 2 {
		t.Fatalf("expected 2 orphans, got %d (%+v)", got.Total, got)
	}
	if got.Open != 1 {
		t.Fatalf("expected 1 open orphan, got %d", got.Open)
	}
	if got.Fraction != 0.5 {
		t.Fatalf("expected fraction 0.5 (2/4 considered), got %v", got.Fraction)
	}
}

func TestComputeCorpusOrphansUsesCenteredAlignment(t *testing.T) {
	tags := []issues.Tag{
		{Name: "tag-a", Embedding: []float64{10, 1, 0, 0, 0, 0, 0}},
		{Name: "tag-b", Embedding: []float64{10, -1, 0, 0, 0, 0, 0}},
		{Name: "tag-c", Embedding: []float64{10, 0, 1, 0, 0, 0, 0}},
		{Name: "tag-d", Embedding: []float64{10, 0, -1, 0, 0, 0, 0}},
		{Name: "tag-e", Embedding: []float64{10, 0, 0, 1, 0, 0, 0}},
	}
	items := []issues.Issue{
		{ID: "aligned-a", Status: issues.StatusOpen, Embedding: []float64{10, 1, 0, 0, 0, 0, 0}, TagScores: []domain.TagRelevance{{Tag: "tag-a", Relevance: 0.9}}},
		{ID: "aligned-b", Status: issues.StatusOpen, Embedding: []float64{10, -1, 0, 0, 0, 0, 0}, TagScores: []domain.TagRelevance{{Tag: "tag-b", Relevance: 0.9}}},
		{ID: "aligned-c", Status: issues.StatusOpen, Embedding: []float64{10, 0, 1, 0, 0, 0, 0}, TagScores: []domain.TagRelevance{{Tag: "tag-c", Relevance: 0.9}}},
		{ID: "aligned-d", Status: issues.StatusOpen, Embedding: []float64{10, 0, -1, 0, 0, 0, 0}, TagScores: []domain.TagRelevance{{Tag: "tag-d", Relevance: 0.9}}},
		{ID: "common-direction-only", Status: issues.StatusOpen, Embedding: []float64{10, 0, 0, 0, 0, 0, 1}, TagScores: []domain.TagRelevance{{Tag: "tag-a", Relevance: 0.1}}},
	}

	got := ComputeCorpusOrphans(items, tags)
	if got.Total != 1 {
		t.Fatalf("expected common-direction-only issue to be counted as an orphan after centering, got %+v", got)
	}
	if got.Open != 1 {
		t.Fatalf("expected 1 open orphan, got %d", got.Open)
	}
	if got.Fraction != 0.2 {
		t.Fatalf("expected fraction 0.2 (1/5 considered), got %v", got.Fraction)
	}
}

func TestComputeCorpusOrphansSkipsIssuesWithoutEmbedding(t *testing.T) {
	tags := []issues.Tag{{Name: "auth", Embedding: []float64{1, 0}}}
	items := []issues.Issue{
		{ID: "1", Status: issues.StatusOpen, Embedding: nil, TagScores: []domain.TagRelevance{}},
	}
	got := ComputeCorpusOrphans(items, tags)
	if got.Total != 0 {
		t.Fatalf("expected 0 orphans when issues lack embeddings, got %d", got.Total)
	}
}

func TestComputeAuthorityMembersCountsCanonicalLinks(t *testing.T) {
	members := []issues.Issue{
		{
			ID: "i1",
			Links: []issues.IssueLink{
				{Type: issues.IssueLinkTypeDuplicateOf, TargetIssueID: "i1"},
				{Type: issues.IssueLinkTypeMergedInto, TargetIssueID: "i1"},
				{Type: issues.IssueLinkTypeRelatedTo, TargetIssueID: "i1"}, // not canonical
			},
		},
		{ID: "i2"},
		{
			ID: "i3",
			Links: []issues.IssueLink{
				{Type: issues.IssueLinkTypeDuplicateOf, TargetIssueID: "other"}, // not inbound
				{Type: issues.IssueLinkTypeDuplicateOf, TargetIssueID: "i3"},
			},
		},
	}
	links, mean := ComputeAuthorityMembers(members)
	if links != 3 {
		t.Fatalf("expected 3 inbound canonical links, got %d", links)
	}
	if mean == nil {
		t.Fatal("expected non-nil mean")
	}
	if *mean <= 0 {
		t.Fatalf("expected positive mean, got %v", *mean)
	}
}

func TestComputeAuthorityMembersEmpty(t *testing.T) {
	links, mean := ComputeAuthorityMembers(nil)
	if links != 0 {
		t.Fatalf("expected 0, got %d", links)
	}
	if mean != nil {
		t.Fatalf("expected nil mean, got %+v", mean)
	}
}

func TestComputeChurnCountsInRegionEvents(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	items := []issues.Issue{
		{ID: "in-region", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}},
		{ID: "outside", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: "billing", Relevance: 0.9}}},
	}
	events := []issues.Event{
		{IssueID: "in-region", Kind: "refinement", CreatedAt: now.Add(-3 * 24 * time.Hour)},
		{IssueID: "in-region", Kind: "split", CreatedAt: now.Add(-1 * 24 * time.Hour)},
		{IssueID: "outside", Kind: "refinement", CreatedAt: now.Add(-2 * 24 * time.Hour)},
	}
	window, _ := ParseTimeWindow("30d", now)
	churn := ComputeChurn(events, items, auth, window)
	if churn == nil || churn.Count != 2 {
		t.Fatalf("expected count 2 (in-region only); got %+v", churn)
	}
}

func TestComputeChurnReturnsNilForAllWindow(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}},
	}
	events := []issues.Event{
		{IssueID: "a", Kind: "refinement", CreatedAt: now.Add(-time.Hour)},
	}
	window, _ := ParseTimeWindow("all", now)
	if got := ComputeChurn(events, items, auth, window); got != nil {
		t.Fatalf("expected nil for all window; got %+v", got)
	}
}

func TestComputeMetricsNilFlowMetricsForAllWindow(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	auth := domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"}
	items := []issues.Issue{
		{Status: issues.StatusOpen, CreatedAt: now.Add(-time.Hour), TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}},
	}
	window, _ := ParseTimeWindow("all", now)
	metrics, ok := ComputeMetrics(items, nil, auth, window, now)
	if !ok {
		t.Fatal("expected metrics for non-empty region")
	}
	if metrics.Growth != nil {
		t.Fatalf("expected Growth nil for all window; got %+v", metrics.Growth)
	}
	if metrics.Closure != nil {
		t.Fatalf("expected Closure nil for all window; got %+v", metrics.Closure)
	}
}

func TestListRegionsWithMetricsIgnoresBelowFloor(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window := domain.TimeWindow{Label: "30d", End: now}
	items := []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{
			{Tag: authTag, Relevance: 0.1},
			{Tag: "billing", Relevance: 0.6},
		}},
	}
	regions := ListRegionsWithMetrics(items, nil, window, now)
	if len(regions) != 1 {
		t.Fatalf("expected 1 region, got %d", len(regions))
	}
	if regions[0].Region.Key.ID != "billing" {
		t.Fatalf("expected billing only; got %q", regions[0].Region.Key.ID)
	}
}
