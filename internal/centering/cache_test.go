package centering

import (
	"context"
	"math"
	"testing"

	"sortit/internal/issues"
)

type stubStore struct {
	items []issues.Issue
	calls int
}

func (s *stubStore) List(context.Context) ([]issues.Issue, error) {
	s.calls++
	return s.items, nil
}

type stubTags struct {
	tags []issues.Tag
}

func (s *stubTags) StoredTags(context.Context) ([]issues.Tag, error) {
	return s.tags, nil
}

type stubRevisions struct {
	rev uint64
}

func (s *stubRevisions) Revision() uint64 { return s.rev }

func corpusIssues() []issues.Issue {
	return []issues.Issue{
		{ID: "a", Embedding: []float64{1, 0}},
		{ID: "b", Embedding: []float64{0, 1}},
		{ID: "c", Embedding: []float64{1, 0}},
		{ID: "d", Embedding: []float64{0, 1}},
		{ID: "e", Embedding: []float64{1, 0}},
		{ID: "no-embedding"},
	}
}

func TestCacheNilSafe(t *testing.T) {
	var c *Cache
	means, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !means.IsZero() {
		t.Fatalf("means = %+v, want zero", means)
	}
}

func TestCacheComputesMeans(t *testing.T) {
	store := &stubStore{items: corpusIssues()}
	c := &Cache{Store: store, Tags: &stubTags{}}

	means, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if len(means.Issue) != 2 {
		t.Fatalf("issue mean = %v, want 2 dims", means.Issue)
	}
	// 3×(1,0) + 2×(0,1) over 5 usable vectors.
	if math.Abs(means.Issue[0]-0.6) > 1e-12 || math.Abs(means.Issue[1]-0.4) > 1e-12 {
		t.Fatalf("issue mean = %v, want [0.6 0.4]", means.Issue)
	}
	// No stored tag embeddings → no tag mean.
	if len(means.Tag) != 0 {
		t.Fatalf("tag mean = %v, want empty", means.Tag)
	}
}

func TestCacheMemoizesByRevision(t *testing.T) {
	store := &stubStore{items: corpusIssues()}
	revisions := &stubRevisions{rev: 7}
	c := &Cache{Store: store, Revisions: revisions}

	if _, err := c.Current(context.Background()); err != nil {
		t.Fatalf("Current: %v", err)
	}
	if _, err := c.Current(context.Background()); err != nil {
		t.Fatalf("Current: %v", err)
	}
	if store.calls != 1 {
		t.Fatalf("store.calls = %d, want 1 (memoized at same revision)", store.calls)
	}

	revisions.rev = 8
	if _, err := c.Current(context.Background()); err != nil {
		t.Fatalf("Current: %v", err)
	}
	if store.calls != 2 {
		t.Fatalf("store.calls = %d, want 2 (recompute after bump)", store.calls)
	}
}

func TestCacheInvalidate(t *testing.T) {
	store := &stubStore{items: corpusIssues()}
	revisions := &stubRevisions{rev: 7}
	c := &Cache{Store: store, Revisions: revisions}

	if _, err := c.Current(context.Background()); err != nil {
		t.Fatalf("Current: %v", err)
	}
	c.Invalidate()
	if _, err := c.Current(context.Background()); err != nil {
		t.Fatalf("Current: %v", err)
	}
	if store.calls != 2 {
		t.Fatalf("store.calls = %d, want 2 after Invalidate", store.calls)
	}
}

func TestCacheWithoutRevisionsAlwaysRecomputes(t *testing.T) {
	store := &stubStore{items: corpusIssues()}
	c := &Cache{Store: store}

	_, _ = c.Current(context.Background())
	_, _ = c.Current(context.Background())
	if store.calls != 2 {
		t.Fatalf("store.calls = %d, want 2 (no revision source)", store.calls)
	}
}
