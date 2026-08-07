// Package ridgedecomp memoizes the full-corpus anchored-ridge decomposition
// (see internal/issuemath/ridge_decomposition.go) by corpus revision, following
// the same pattern as internal/ridgelambda and internal/centering.
//
// The decomposition — the Gram build O(K²·D) plus one O(K²) solve per issue —
// is a corpus-level product artifact, not a per-query estimate, so it is
// computed once over the full corpus and reused across searches, explores, and
// person recommendations. Handlers pass the cached bundle to
// issuemap.WithRidgeDecomposition (and the explore/person equivalents); the
// ranker itself never runs the solve, because the candidate set it sees is
// usually a retrieved subset.
//
// This cache lives in the *ranking* λ regime (GCV-selected unscored penalty,
// via internal/ridgelambda). The tag-health / drift sweep uses the fixed loose
// penalty (conventions §6) and deliberately does NOT read this cache.
package ridgedecomp

import (
	"context"
	"sync"

	"sortit/internal/centering"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
	"sortit/internal/ridgelambda"
	"sortit/internal/scoring"
)

// IssueLister is the minimal read interface the cache needs. Production wires
// the issue store; tests can supply a stub.
type IssueLister interface {
	List(ctx context.Context) ([]issues.Issue, error)
}

// TagSource exposes the stored tag catalog. Production wires
// *tags.CatalogService.
type TagSource interface {
	StoredTags(ctx context.Context) ([]issues.Tag, error)
}

// RevisionSource exposes the monotonically increasing corpus revision so the
// cache invalidates on writes. Optional — when nil, the cache always recomputes.
type RevisionSource interface {
	Revision() uint64
}

// Cache memoizes the full-corpus ridge decomposition by revision.
type Cache struct {
	Store     IssueLister
	Tags      TagSource
	Revisions RevisionSource
	// Centering supplies the revision-cached corpus means so the decomposition
	// runs in the same centered space as the ranker. Optional — when nil the
	// embeddings are unit-normalized but not centered. Ignored when Uncentered
	// is set.
	Centering *centering.Cache
	// Uncentered solves the decomposition on uncentered inputs (unit-normalized,
	// no corpus-mean subtraction) and stamps zero Means on the bundle, so query
	// and profile embeddings decompose into the same uncentered space. This is
	// the ranking-regime default (WP-304: on real embedding geometry the
	// centered tag-space ridge loses to rank-1; the uncentered one wins).
	// Structure consumers (themes) keep their own centered instance — the two
	// regimes must not share a cache. Lambda should be an Uncentered
	// ridgelambda cache so the penalty is selected on the same geometry it
	// parameterizes.
	Uncentered bool
	// Lambda supplies the GCV-selected unscored penalty that parameterizes the
	// ranking-regime solve. Required: without it the cache degrades to
	// "not available" and callers fall back to rank-1.
	Lambda *ridgelambda.Cache

	mu           sync.Mutex
	revision     uint64
	decomp       *issuemath.CorpusRidgeDecomposition
	ok           bool
	computed     bool
	computeCount int // number of actual computes; asserted by the concurrency test
}

// Current returns the full-corpus decomposition at the current revision,
// recomputing it when the cache is empty or the revision has bumped. The
// boolean is false when the corpus is too small or degenerate to decompose, or
// when the λ cache has no penalty — callers then fall back to the rank-1
// similarity model. A nil cache or store yields (nil, false, nil).
//
// The whole call is guarded by the mutex, so a burst of first requests after a
// revision bump computes the (expensive) decomposition exactly once; the rest
// block and read the memoized result. The returned pointer is immutable after
// compute, so concurrent readers are safe.
func (c *Cache) Current(ctx context.Context) (*issuemath.CorpusRidgeDecomposition, bool, error) {
	if c == nil || c.Store == nil {
		return nil, false, nil
	}

	rev := c.currentRevision()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.computed && rev != 0 && rev == c.revision {
		return c.decomp, c.ok, nil
	}

	decomp, ok, err := c.compute(ctx)
	if err != nil {
		return nil, false, err
	}

	c.decomp = decomp
	c.ok = ok
	c.revision = rev
	c.computed = true
	return decomp, ok, nil
}

func (c *Cache) compute(ctx context.Context) (*issuemath.CorpusRidgeDecomposition, bool, error) {
	if c.Lambda == nil {
		return nil, false, nil
	}

	items, err := c.Store.List(ctx)
	if err != nil {
		return nil, false, err
	}
	var storeTags []issues.Tag
	if c.Tags != nil {
		if storeTags, err = c.Tags.StoredTags(ctx); err != nil {
			return nil, false, err
		}
	}
	if len(items) < scoring.MinDecompositionIssues || !hasTagEmbedding(storeTags) {
		return nil, false, nil
	}

	lambda, ok, err := c.Lambda.Current(ctx)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	means, err := c.corpusMeans(ctx)
	if err != nil {
		return nil, false, err
	}

	c.computeCount++
	decomp := issuemap.ComputeCorpusRidgeDecomposition(items, storeTags, means,
		scoring.RidgeAnchorLambdaScored, lambda)
	if !decomp.Decomposed() {
		return nil, false, nil
	}
	return &decomp, true, nil
}

// corpusMeans returns the means the decomposition is solved with. In the
// uncentered regime (and when no centering cache is wired) they are zero, so
// the solve runs on unit-normalized, uncentered embeddings and the bundle's
// Means keep external query/profile embeddings in that same space. Otherwise
// the revision-cached corpus means match the centered ranker space.
func (c *Cache) corpusMeans(ctx context.Context) (issuemath.CorpusMeans, error) {
	if c.Uncentered || c.Centering == nil {
		return issuemath.CorpusMeans{}, nil
	}
	return c.Centering.Current(ctx)
}

func (c *Cache) currentRevision() uint64 {
	if c == nil || c.Revisions == nil {
		return 0
	}
	return c.Revisions.Revision()
}

func hasTagEmbedding(storeTags []issues.Tag) bool {
	for _, tag := range storeTags {
		if len(tag.Embedding) > 0 {
			return true
		}
	}
	return false
}
