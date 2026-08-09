package themes

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/ridgedecomp"
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

// themeTags is the shared two-tag catalog with embeddings on orthogonal axes.
func themeTags() []issues.Tag {
	return []issues.Tag{
		{Name: "alpha", Embedding: unit([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unit([]float64{0, 1, 0, 0})},
	}
}

// clusteredIssues builds n anchored, embedded issues split between the alpha and
// beta clusters, with deterministic per-issue jitter. IDs are zero-padded so an
// ID sort is a stable order. Every issue satisfies the participation rule.
func clusteredIssues(n int) []issues.Issue {
	items := make([]issues.Issue, 0, n)
	for i := 0; i < n; i++ {
		jitter := float64(i%5) * 0.01
		var emb []float64
		tag := "alpha"
		if i%2 == 0 {
			emb = unit([]float64{0.9 + jitter, 0.15 + jitter, 0.1, 0})
		} else {
			tag = "beta"
			emb = unit([]float64{0.15 + jitter, 0.9 + jitter, 0.1, 0})
		}
		items = append(items, issues.Issue{
			ID:        fmt.Sprintf("issue-%03d", i),
			Status:    issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: tag, Relevance: 0.8 + jitter}},
			Embedding: emb,
		})
	}
	return items
}

// newCache wires a themes cache over the given corpus, sharing one store and
// revision source with the decomposition and λ caches it consumes.
func newCache(items []issues.Issue, tags []issues.Tag, rev *stubRevisions) *Cache {
	store := &stubStore{items: items}
	tagSrc := &stubTags{tags: tags}
	lambda := &ridgelambda.Cache{Store: store, Tags: tagSrc, Revisions: rev}
	decomp := &ridgedecomp.Cache{Store: store, Tags: tagSrc, Revisions: rev, Lambda: lambda}
	return &Cache{Decomp: decomp, Store: store, Revisions: rev}
}

func TestCacheNilSafe(t *testing.T) {
	var c *Cache
	res, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if ok || !reflect.DeepEqual(res, Result{}) {
		t.Fatalf("nil cache should yield (zero, false), got (%+v, %v)", res, ok)
	}
}

// TestCacheComputesAndMemoizesByRevision covers the two core cache behaviors:
// a fresh corpus factorizes once, the same revision is memoized (no recompute),
// and a revision bump recomputes.
func TestCacheComputesAndMemoizesByRevision(t *testing.T) {
	items := clusteredIssues(24)
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, themeTags(), rev)

	res, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !ok {
		t.Fatal("expected a usable factorization for a 24-issue corpus")
	}
	if len(res.Themes) == 0 {
		t.Fatal("expected at least one theme")
	}
	if res.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", res.Revision)
	}
	if res.Participating != 24 {
		t.Fatalf("expected 24 participating issues, got %d", res.Participating)
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
	res2, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current (new revision): %v", err)
	}
	if !ok || res2.Revision != 2 {
		t.Fatalf("expected recompute at revision 2, got ok=%v rev=%d", ok, res2.Revision)
	}
	if c.computeCount != 2 {
		t.Fatalf("expected recompute after revision bump, computeCount=%d", c.computeCount)
	}
}

// TestParticipationRule asserts the corpus split: no-embedding issues are
// excluded, no-anchor issues are excluded, and closed issues participate.
func TestParticipationRule(t *testing.T) {
	items := clusteredIssues(20)
	// One participating issue is closed — it must still count (themes describe
	// the corpus, not the backlog).
	items[0].Status = issues.StatusClosed
	closedID := items[0].ID

	// An embedded issue with no anchored tag → excluded as no-anchor.
	items = append(items, issues.Issue{
		ID:        "no-anchor",
		Status:    issues.StatusOpen,
		Embedding: unit([]float64{0.5, 0.5, 0.2, 0}),
	})
	// An anchored issue with no embedding → excluded as no-embedding.
	items = append(items, issues.Issue{
		ID:        "no-embed",
		Status:    issues.StatusOpen,
		TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
	})

	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, themeTags(), rev)

	res, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !ok {
		t.Fatal("expected a usable factorization")
	}
	if res.Participating != 20 {
		t.Fatalf("expected 20 participating (incl. the closed issue), got %d", res.Participating)
	}
	if res.ExcludedNoEmbedding != 1 {
		t.Fatalf("expected 1 no-embedding exclusion, got %d", res.ExcludedNoEmbedding)
	}
	if res.ExcludedNoAnchor != 1 {
		t.Fatalf("expected 1 no-anchor exclusion, got %d", res.ExcludedNoAnchor)
	}

	// The closed issue must appear among the factorized issue rows.
	found := false
	for _, it := range res.IssueThemes {
		if it.IssueID == closedID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("closed issue %s should participate but is absent from IssueThemes", closedID)
	}
}

// TestDegenerateFloorReturnsFalse asserts a corpus that decomposes but has fewer
// than minThemeParticipants participating issues yields (zero, false): themes on
// too few issues are noise.
func TestDegenerateFloorReturnsFalse(t *testing.T) {
	if minThemeParticipants <= scoring.MinDecompositionIssues {
		t.Fatalf("test assumes the theme floor (%d) exceeds the decomposition floor", minThemeParticipants)
	}
	items := clusteredIssues(minThemeParticipants - 1) // decomposes, but below the theme floor
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, themeTags(), rev)

	res, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if ok || !reflect.DeepEqual(res, Result{}) {
		t.Fatalf("below-floor corpus should yield (zero, false), got (%+v, %v)", res, ok)
	}
}

// TestDecompositionUnavailableReturnsFalse asserts the cache degrades to
// (zero, false) when the decomposition itself is unavailable (corpus too small).
func TestDecompositionUnavailableReturnsFalse(t *testing.T) {
	items := clusteredIssues(3) // below scoring.MinDecompositionIssues
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, themeTags(), rev)

	res, ok, err := c.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if ok || !reflect.DeepEqual(res, Result{}) {
		t.Fatalf("no decomposition should yield (zero, false), got (%+v, %v)", res, ok)
	}
}

// TestDeterministicAcrossCaches asserts two independent computes over the same
// corpus at the same revision are bit-identical — determinism carries end to end
// through the adapter and NMF.
func TestDeterministicAcrossCaches(t *testing.T) {
	items := clusteredIssues(24)
	tags := themeTags()

	rev1 := &stubRevisions{}
	rev1.rev.Store(1)
	res1, ok1, err := newCache(items, tags, rev1).Current(context.Background())
	if err != nil || !ok1 {
		t.Fatalf("first compute: ok=%v err=%v", ok1, err)
	}

	rev2 := &stubRevisions{}
	rev2.rev.Store(1)
	res2, ok2, err := newCache(items, tags, rev2).Current(context.Background())
	if err != nil || !ok2 {
		t.Fatalf("second compute: ok=%v err=%v", ok2, err)
	}

	if !reflect.DeepEqual(res1, res2) {
		t.Fatal("two computes over the same corpus at the same revision must be bit-identical")
	}
}

// TestSingleFlightUnderConcurrency exercises Current from many goroutines across
// a revision bump under -race, asserting exactly one compute per revision (the
// compute is guarded by the cache mutex).
func TestSingleFlightUnderConcurrency(t *testing.T) {
	items := clusteredIssues(24)
	rev := &stubRevisions{}
	rev.rev.Store(1)
	c := newCache(items, themeTags(), rev)

	const goroutines = 8
	hammer := func() {
		var wg sync.WaitGroup
		for range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res, ok, err := c.Current(context.Background())
				if err != nil {
					t.Errorf("Current: %v", err)
					return
				}
				if !ok || len(res.Themes) == 0 {
					t.Errorf("expected a usable factorization")
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
