package tagcooccurrence

import (
	"context"
	"sync/atomic"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

type stubLister struct {
	items []issues.Issue
	calls int32
}

func (s *stubLister) List(_ context.Context) ([]issues.Issue, error) {
	atomic.AddInt32(&s.calls, 1)
	return s.items, nil
}

type stubRevision struct {
	v uint64
}

func (s *stubRevision) Revision() uint64 { return s.v }

func TestCacheCachesByRevision(t *testing.T) {
	store := &stubLister{items: []issues.Issue{
		{ID: "a", TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}},
	}}
	rev := &stubRevision{v: 1}
	cache := &Cache{Store: store, Revisions: rev}

	for range 3 {
		if _, err := cache.Current(context.Background()); err != nil {
			t.Fatalf("Current: %v", err)
		}
	}
	if got := atomic.LoadInt32(&store.calls); got != 1 {
		t.Fatalf("expected 1 store call (cached); got %d", got)
	}

	rev.v = 2
	if _, err := cache.Current(context.Background()); err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got := atomic.LoadInt32(&store.calls); got != 2 {
		t.Fatalf("expected 2 store calls after revision bump; got %d", got)
	}
}

func TestCacheInvalidateForcesRecompute(t *testing.T) {
	store := &stubLister{items: []issues.Issue{
		{ID: "a", TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}},
	}}
	cache := &Cache{Store: store, Revisions: &stubRevision{v: 1}}

	if _, err := cache.Current(context.Background()); err != nil {
		t.Fatalf("Current: %v", err)
	}
	cache.Invalidate()
	if _, err := cache.Current(context.Background()); err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got := atomic.LoadInt32(&store.calls); got != 2 {
		t.Fatalf("expected 2 store calls after Invalidate; got %d", got)
	}
}

func TestCacheLookupPairsReturnsFalseForUnknownTag(t *testing.T) {
	store := &stubLister{items: []issues.Issue{
		{ID: "a", TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}}},
	}}
	cache := &Cache{Store: store, Revisions: &stubRevision{v: 1}}

	_, ok, err := cache.LookupPairs(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("LookupPairs: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for unknown tag")
	}
}

func TestCacheLookupPairsReturnsKnownTag(t *testing.T) {
	store := &stubLister{items: []issues.Issue{
		{ID: "a", TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.9},
			{Tag: "tokens", Relevance: 0.7},
		}},
		{ID: "b", TagScores: []domain.TagRelevance{
			{Tag: "auth", Relevance: 0.8},
		}},
	}}
	cache := &Cache{Store: store, Revisions: &stubRevision{v: 1}}

	stats, ok, err := cache.LookupPairs(context.Background(), "AUTH")
	if err != nil {
		t.Fatalf("LookupPairs: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if stats.Tag != "auth" || stats.Count != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}
