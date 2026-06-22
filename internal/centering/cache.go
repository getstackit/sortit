// Package centering memoizes the corpus embedding means used for runtime
// corpus-mean centering (see internal/issuemath/centering.go). Persisted
// embeddings are never rewritten; the means are recomputed from the store
// and cached by corpus revision, following the same pattern as
// internal/tagcooccurrence.
//
// The cached means matter most at search time: the candidate set handed to
// the ranker may be a retrieved subset whose own mean points toward the
// query neighborhood. Centering with that subset mean would subtract exactly
// the signal being searched for, so consumers center with these stored
// full-corpus means instead.
package centering

import (
	"context"
	"sync"

	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/vectors"
)

// IssueLister is the minimal read interface the cache needs to recompute the
// issue-corpus mean. Production wires the issue store; tests can supply a stub.
type IssueLister interface {
	List(ctx context.Context) ([]issues.Issue, error)
}

// TagSource exposes the stored tag catalog for the tag-catalog mean.
// Production wires *tags.CatalogService. Optional — when nil, only the
// issue mean is computed.
type TagSource interface {
	StoredTags(ctx context.Context) ([]issues.Tag, error)
}

// RevisionSource exposes the monotonically increasing corpus revision so the
// cache invalidates on writes. Optional — when nil, the cache always recomputes.
type RevisionSource interface {
	Revision() uint64
}

// Cache memoizes the corpus embedding means by revision.
type Cache struct {
	Store     IssueLister
	Tags      TagSource
	Revisions RevisionSource

	mu       sync.Mutex
	revision uint64
	means    *issuemath.CorpusMeans
}

// Current returns the corpus means at the current revision, recomputing them
// when the cache is empty or the revision has bumped. A nil cache or store
// yields zero means, which downstream centering treats as "leave uncentered".
func (c *Cache) Current(ctx context.Context) (issuemath.CorpusMeans, error) {
	if c == nil || c.Store == nil {
		return issuemath.CorpusMeans{}, nil
	}

	rev := c.currentRevision()

	c.mu.Lock()
	if c.means != nil && rev != 0 && rev == c.revision {
		out := *c.means
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	items, err := c.Store.List(ctx)
	if err != nil {
		return issuemath.CorpusMeans{}, err
	}
	var storeTags []issues.Tag
	if c.Tags != nil {
		storeTags, err = c.Tags.StoredTags(ctx)
		if err != nil {
			return issuemath.CorpusMeans{}, err
		}
	}
	computed := computeMeans(items, storeTags)

	c.mu.Lock()
	c.means = &computed
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
	c.means = nil
	c.revision = 0
}

func (c *Cache) currentRevision() uint64 {
	if c == nil || c.Revisions == nil {
		return 0
	}
	return c.Revisions.Revision()
}

// computeMeans mirrors the runtime corpus boundary: issue embeddings are
// unit-normalized before the mean is taken (the runtime normalizes stored
// embeddings on load), tag embeddings are used as stored.
func computeMeans(items []issues.Issue, storeTags []issues.Tag) issuemath.CorpusMeans {
	issueEmbeddings := make(map[string][]float64, len(items))
	for _, item := range items {
		if len(item.Embedding) == 0 {
			continue
		}
		issueEmbeddings[item.ID] = vectors.NormalizedCopy(item.Embedding)
	}

	tagEmbeddings := make(map[string][]float64, len(storeTags))
	for _, tag := range storeTags {
		if len(tag.Embedding) == 0 {
			continue
		}
		tagEmbeddings[tag.Name] = tag.Embedding
	}

	return issuemath.ComputeCorpusMeans(issueEmbeddings, tagEmbeddings)
}
