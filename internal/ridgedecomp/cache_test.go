package ridgedecomp

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"sortit/internal/centering"
	"sortit/internal/domain"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
	"sortit/internal/ridgelambda"
	"sortit/internal/scoring"
)

type stubStore struct {
	items []issues.Issue
}

func (s *stubStore) List(context.Context) ([]issues.Issue, error) { return s.items, nil }

type stubTags struct {
	tags []issues.Tag
}

func (s *stubTags) StoredTags(context.Context) ([]issues.Tag, error) { return s.tags, nil }

type stubRevisions struct {
	rev atomic.Uint64
}

func (s *stubRevisions) Revision() uint64 { return s.rev.Load() }

func unit(v []float64) []float64 {
	var mag float64
	for _, x := range v {
		mag += x * x
	}
	if mag == 0 {
		return v
	}
	mag = math.Sqrt(mag)
	out := make([]float64, len(v))
	for i := range v {
		out[i] = v[i] / mag
	}
	return out
}

// twoClusterCorpus is large enough to decompose and has separable tag
// structure, so the GCV penalty and the decomposition both succeed.
func twoClusterCorpus() ([]issues.Issue, []issues.Tag) {
	tags := []issues.Tag{
		{Name: "alpha", Embedding: unit([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unit([]float64{0, 1, 0, 0})},
	}
	items := []issues.Issue{
		{ID: "a1", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}}, Embedding: unit([]float64{0.9, 0.2, 0.1, 0})},
		{ID: "a2", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.8}}, Embedding: unit([]float64{0.95, 0.1, 0.2, 0})},
		{ID: "a3", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.85}}, Embedding: unit([]float64{0.92, 0.15, 0.1, 0})},
		{ID: "b1", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.9}}, Embedding: unit([]float64{0.1, 0.9, 0.2, 0})},
		{ID: "b2", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.8}}, Embedding: unit([]float64{0.2, 0.95, 0.1, 0})},
		{ID: "b3", Status: issues.StatusOpen, TagScores: []domain.TagRelevance{{Tag: "beta", Relevance: 0.85}}, Embedding: unit([]float64{0.15, 0.92, 0.1, 0})},
	}
	return items, tags
}

func newCache(items []issues.Issue, tags []issues.Tag, rev *stubRevisions) *Cache {
	store := &stubStore{items: items}
	tagSrc := &stubTags{tags: tags}
	return &Cache{
		Store:     store,
		Tags:      tagSrc,
		Revisions: rev,
		Lambda: &ridgelambda.Cache{
			Store:     store,
			Tags:      tagSrc,
			Revisions: rev,
		},
	}
}

func TestCacheNilSafe(t *testing.T) {
	var c *Cache
	decomp, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if ok || decomp != nil {
		t.Fatalf("nil cache should yield (nil, false), got (%v, %v)", decomp, ok)
	}
}

func TestCacheDecomposesAndMemoizesByRevision(t *testing.T) {
	items, tags := twoClusterCorpus()
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, tags, rev)

	decomp, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !ok || decomp == nil || !decomp.Decomposed() {
		t.Fatalf("expected a usable decomposition, got (%v, %v)", decomp, ok)
	}
	if _, found := decomp.Decomposition.VectorsFor("a1"); !found {
		t.Fatal("expected cached vectors for issue a1")
	}
	if c.computeCount != 1 {
		t.Fatalf("expected 1 compute, got %d", c.computeCount)
	}

	// Same revision → memoized, no recompute.
	if _, _, err := c.Current(context.Background()); err != nil {
		t.Fatalf("Current (cached): %v", err)
	}
	if c.computeCount != 1 {
		t.Fatalf("expected memoized result at the same revision, computeCount=%d", c.computeCount)
	}

	// Bump revision → recompute.
	rev.rev.Store(2)
	if _, _, err := c.Current(context.Background()); err != nil {
		t.Fatalf("Current (new revision): %v", err)
	}
	if c.computeCount != 2 {
		t.Fatalf("expected recompute after revision bump, computeCount=%d", c.computeCount)
	}
}

func TestCacheRejectsSmallCorpus(t *testing.T) {
	items, tags := twoClusterCorpus()
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items[:3], tags, rev) // below MinDecompositionIssues

	decomp, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if ok || decomp != nil {
		t.Fatalf("small corpus should yield (nil, false), got (%v, %v)", decomp, ok)
	}
}

func TestCacheRejectsCorpusWithoutTagEmbeddings(t *testing.T) {
	items, _ := twoClusterCorpus()
	tags := []issues.Tag{{Name: "alpha"}, {Name: "beta"}} // no embeddings
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, tags, rev)

	decomp, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if ok || decomp != nil {
		t.Fatalf("corpus without tag embeddings should yield (nil, false), got (%v, %v)", decomp, ok)
	}
}

// TestCacheSingleFlightUnderConcurrency exercises Current from many goroutines
// across a revision bump under -race, asserting exactly one compute per
// revision (the compute is guarded by the cache mutex).
func TestCacheSingleFlightUnderConcurrency(t *testing.T) {
	items, tags := twoClusterCorpus()
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, tags, rev)

	const goroutines = 8
	hammer := func() {
		var wg sync.WaitGroup
		for range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				decomp, ok, err := c.Current(context.Background())
				if err != nil {
					t.Errorf("Current: %v", err)
					return
				}
				if !ok || decomp == nil {
					t.Errorf("expected a usable decomposition")
					return
				}
				// Concurrent reads of the shared cached artifact.
				if _, found := decomp.Decomposition.VectorsFor("a1"); !found {
					t.Errorf("expected cached vectors for a1")
				}
			}()
		}
		wg.Wait()
	}

	hammer()
	if c.computeCount != 1 {
		t.Fatalf("expected exactly 1 compute at revision 1, got %d", c.computeCount)
	}

	rev.rev.Store(2)
	hammer()
	if c.computeCount != 2 {
		t.Fatalf("expected exactly 2 computes across the revision bump, got %d", c.computeCount)
	}
}

// TestCacheUncenteredRegime asserts the ranking regime introduced by WP-304:
// an Uncentered cache solves on unit-normalized, uncentered inputs, stamps
// zero Means on the bundle (so external query/profile embeddings stay
// uncentered when centered with them), and ignores a wired centering cache.
func TestCacheUncenteredRegime(t *testing.T) {
	items, tags := twoClusterCorpus()
	store := &stubStore{items: items}
	tagSrc := &stubTags{tags: tags}
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := &Cache{
		Store:      store,
		Tags:       tagSrc,
		Revisions:  rev,
		Uncentered: true,
		// A wired centering cache must be ignored in the uncentered regime.
		Centering: &centering.Cache{Store: store},
		Lambda:    &ridgelambda.Cache{Store: store, Tags: tagSrc, Revisions: rev, Uncentered: true},
	}

	decomp, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !ok || decomp == nil || !decomp.Decomposed() {
		t.Fatalf("expected a usable uncentered decomposition, got (%v, %v)", decomp, ok)
	}
	if !decomp.Means.IsZero() {
		t.Fatalf("uncentered bundle must carry zero Means, got issue=%v tag=%v", decomp.Means.Issue, decomp.Means.Tag)
	}

	// The solve must match an explicit zero-means corpus decomposition at the
	// same λ — the uncentered reference.
	lambda, lok, err := c.Lambda.Current(context.Background())
	if err != nil || !lok {
		t.Fatalf("uncentered λ unavailable: (%v, %v, %v)", lambda, lok, err)
	}
	want := issuemap.ComputeCorpusRidgeDecomposition(items, tags, issuemath.CorpusMeans{},
		scoring.RidgeAnchorLambdaScored, lambda)
	got, found := decomp.Decomposition.VectorsFor("a1")
	wantVec, wantFound := want.Decomposition.VectorsFor("a1")
	if !found || !wantFound {
		t.Fatalf("expected vectors for a1 in both decompositions (cache=%v reference=%v)", found, wantFound)
	}
	if len(got.Loading) != len(wantVec.Loading) {
		t.Fatalf("loading length mismatch: %d vs %d", len(got.Loading), len(wantVec.Loading))
	}
	for i := range got.Loading {
		if math.Abs(got.Loading[i]-wantVec.Loading[i]) > 1e-12 {
			t.Fatalf("loading[%d] = %v, want %v (uncentered reference)", i, got.Loading[i], wantVec.Loading[i])
		}
	}
}
