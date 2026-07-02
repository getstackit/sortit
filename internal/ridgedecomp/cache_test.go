package ridgedecomp

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"sortit/internal/issues"
	"sortit/internal/ridgelambda"
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
		{ID: "a1", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.9}}, Embedding: unit([]float64{0.9, 0.2, 0.1, 0})},
		{ID: "a2", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.8}}, Embedding: unit([]float64{0.95, 0.1, 0.2, 0})},
		{ID: "a3", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "alpha", Relevance: 0.85}}, Embedding: unit([]float64{0.92, 0.15, 0.1, 0})},
		{ID: "b1", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "beta", Relevance: 0.9}}, Embedding: unit([]float64{0.1, 0.9, 0.2, 0})},
		{ID: "b2", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "beta", Relevance: 0.8}}, Embedding: unit([]float64{0.2, 0.95, 0.1, 0})},
		{ID: "b3", Status: issues.StatusOpen, TagScores: []issues.TagRelevance{{Tag: "beta", Relevance: 0.85}}, Embedding: unit([]float64{0.15, 0.92, 0.1, 0})},
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
