package regions

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

type stubStore struct {
	items  []issues.Issue
	events []issues.Event
	calls  int32
	err    error
}

func (s *stubStore) List(_ context.Context) ([]issues.Issue, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.err != nil {
		return nil, s.err
	}
	return s.items, nil
}

func (s *stubStore) ListLifecycleEvents(_ context.Context, _ []string, _, _ time.Time) ([]issues.Event, error) {
	return s.events, nil
}

type stubRevision struct {
	v uint64
}

func (s *stubRevision) Revision() uint64 { return s.v }

func TestLoaderCachesByRevision(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window := domain.TimeWindow{Label: "30d", End: now}
	store := &stubStore{items: []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.9}}},
	}}
	rev := &stubRevision{v: 1}
	loader := &Loader{Store: store, Revisions: rev}

	for range 3 {
		if _, err := loader.List(context.Background(), window, now); err != nil {
			t.Fatalf("List: %v", err)
		}
	}
	if got := atomic.LoadInt32(&store.calls); got != 1 {
		t.Fatalf("expected 1 store call (cached); got %d", got)
	}

	rev.v = 2
	if _, err := loader.List(context.Background(), window, now); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := atomic.LoadInt32(&store.calls); got != 2 {
		t.Fatalf("expected 2 store calls (revision bumped); got %d", got)
	}
}

func TestLoaderInvalidateClearsCache(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window := domain.TimeWindow{Label: "30d", End: now}
	store := &stubStore{items: []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.9}}},
	}}
	loader := &Loader{Store: store, Revisions: &stubRevision{v: 1}}

	if _, err := loader.List(context.Background(), window, now); err != nil {
		t.Fatalf("List: %v", err)
	}
	loader.Invalidate()
	if _, err := loader.List(context.Background(), window, now); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := atomic.LoadInt32(&store.calls); got != 2 {
		t.Fatalf("expected 2 store calls after invalidate; got %d", got)
	}
}

func TestLoaderGetReturnsRegion(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	window := domain.TimeWindow{Label: "30d", End: now}
	store := &stubStore{items: []issues.Issue{
		{ID: "a", Status: issues.StatusOpen, CreatedAt: now, TagScores: []domain.TagRelevance{{Tag: authTag, Relevance: 0.9}}},
	}}
	loader := &Loader{Store: store}

	region, ok, err := loader.Get(context.Background(), domain.RegionKey{Kind: domain.RegionKindTag, ID: "Auth"}, window, now)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("expected region to be found")
	}
	if region.Region.Key.ID != authTag {
		t.Fatalf("expected key auth, got %q", region.Region.Key.ID)
	}
}

func TestLoaderGetRejectsUnsupportedKind(t *testing.T) {
	loader := &Loader{Store: &stubStore{}}
	_, ok, err := loader.Get(context.Background(), domain.RegionKey{Kind: domain.RegionKindCluster, ID: "x"}, domain.TimeWindow{}, time.Now())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for cluster kind")
	}
}

func TestLoaderListPropagatesStoreError(t *testing.T) {
	boom := errors.New("boom")
	loader := &Loader{Store: &stubStore{err: boom}}
	_, err := loader.List(context.Background(), domain.TimeWindow{Label: "30d"}, time.Now())
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
}
