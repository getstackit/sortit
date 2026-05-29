package tagcooccurrence

import (
	"context"
	"sync"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

// IssueLister is the minimal read interface the cache needs to recompute the
// projection. Production wires *issues.PostgresStore; tests can supply a stub.
type IssueLister interface {
	List(ctx context.Context) ([]issues.Issue, error)
}

// RevisionSource exposes the monotonically increasing corpus revision so the
// cache invalidates on writes. Optional — when nil, the cache always recomputes.
type RevisionSource interface {
	Revision() uint64
}

// Cache memoizes the corpus-wide projection by (revision). The projection is
// surprisingly cheap to compute (linear in tags × tags) but pulling the full
// issue list once per debug request is wasteful, and downstream consumers
// (Tier 2 coherence, search anti-correlation) want a stable lookup point.
type Cache struct {
	Store     IssueLister
	Revisions RevisionSource

	mu       sync.Mutex
	revision uint64
	stats    *Stats
}

// Current returns the projection at the current revision, recomputing it
// when the cache is empty or the revision has bumped.
func (c *Cache) Current(ctx context.Context) (Stats, error) {
	if c == nil || c.Store == nil {
		return Stats{}, nil
	}

	rev := c.currentRevision()

	c.mu.Lock()
	if c.stats != nil && rev != 0 && rev == c.revision {
		out := *c.stats
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	items, err := c.Store.List(ctx)
	if err != nil {
		return Stats{}, err
	}
	computed := Compute(items)

	c.mu.Lock()
	c.stats = &computed
	c.revision = rev
	c.mu.Unlock()

	return computed, nil
}

// Invalidate forces a recompute on the next Current call.
func (c *Cache) Invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats = nil
	c.revision = 0
}

func (c *Cache) currentRevision() uint64 {
	if c == nil || c.Revisions == nil {
		return 0
	}
	return c.Revisions.Revision()
}

// LookupPairs returns the per-tag anti-correlation pairs for the given tag
// at the current projection. Returns (TagStats{}, false) when the tag has
// no observations in the corpus.
func (c *Cache) LookupPairs(ctx context.Context, tag string) (TagStats, bool, error) {
	stats, err := c.Current(ctx)
	if err != nil {
		return TagStats{}, false, err
	}
	t, ok := stats.LookupTag(domain.NormalizeTagName(tag))
	return t, ok, nil
}
