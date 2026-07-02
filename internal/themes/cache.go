// Package themes turns the pure NMF library internal/issuethemes into a
// consumable corpus artifact: a revision-keyed cache that reads full-corpus
// ridge loadings from the decomposition cache (internal/ridgedecomp), maps them
// through the theme participation rule, and memoizes the factorization per
// corpus revision — the same shape as internal/ridgelambda and
// internal/ridgedecomp.
//
// The heavy work (the per-issue ridge solve) is paid once by the decomposition
// cache; NMF over the resulting loadings is milliseconds, so like the two
// sibling caches this one holds only in-memory per-revision state and returns an
// explicit "not available" signal ((zero, false)) for degenerate corpora rather
// than an error. Durable theme snapshots are a later need (theme drift over
// time; identity persistence across restarts) deferred to Stage 4, not this WP.
//
// issuethemes stays a pure, caller-free library; all wiring lives here. This
// package adds no API endpoints and no server wiring — WP-204 builds the debug
// API on top of Current.
package themes

import (
	"context"
	"sync"

	"sortit/internal/issues"
	"sortit/internal/issuethemes"
	"sortit/internal/ridgedecomp"
)

// defaultThemeCount mirrors issuethemes' K default (issuethemes.Build with a
// zero ThemeCount factorizes into 8 components). It is duplicated here only so
// the degenerate floor below can be expressed in terms of K.
const defaultThemeCount = 8

// minThemeParticipants is the floor below which NMF themes are noise. With
// fewer than 2·K participating issues there are not enough rows to separate K
// components — the factorization overfits and "themes" become per-issue
// artifacts — so the cache returns (zero, false) and callers show nothing. This
// is the theme analog of scoring.MinDecompositionIssues.
const minThemeParticipants = 2 * defaultThemeCount

// IssueLister is the minimal read interface the cache needs to apply the
// participation rule (it reads each issue's TagScores). Production wires the
// issue store; tests can supply a stub. The decomposition cache holds the
// loadings but not the per-issue anchors, so the theme cache reads issues
// directly — pointed at the same store, so both see the same revision.
type IssueLister interface {
	List(ctx context.Context) ([]issues.Issue, error)
}

// RevisionSource exposes the monotonically increasing corpus revision so the
// cache invalidates on writes. Optional — when nil, the cache always recomputes.
type RevisionSource interface {
	Revision() uint64
}

// Result wraps the pure factorization with the metadata the debug API surfaces:
// the revision it was computed at and the corpus participation split. WP-203
// extends this wrapper with theme identity state (stable IDs, mint/retire
// diagnostics); it is the stable seam those IDs attach to, kept separate from
// the pure issuethemes.Result so the library stays caller-free.
type Result struct {
	issuethemes.Result
	// Revision is the corpus revision the factorization was computed at.
	Revision uint64
	// Participating is the number of issues that fed the NMF (usable embedding
	// AND at least one anchored tag).
	Participating int
	// ExcludedNoEmbedding counts issues dropped for lacking a usable embedding.
	ExcludedNoEmbedding int
	// ExcludedNoAnchor counts issues with a usable embedding but no anchored tag.
	ExcludedNoAnchor int
}

// Cache memoizes the corpus theme factorization by revision.
type Cache struct {
	// Decomp is the revision-keyed full-corpus ridge decomposition this cache
	// consumes. Required: without a usable decomposition there are no loadings
	// to factorize, and Current degrades to (zero, false).
	Decomp *ridgedecomp.Cache
	// Store supplies per-issue TagScores for the participation rule. Required.
	Store IssueLister
	// Revisions is the shared corpus revision source. Optional — when nil the
	// cache always recomputes.
	Revisions RevisionSource

	mu           sync.Mutex
	revision     uint64
	result       Result
	ok           bool
	computed     bool
	computeCount int // number of actual computes; asserted by the concurrency test
}

// Current returns the theme factorization at the current revision, recomputing
// it when the cache is empty or the revision has bumped. The boolean is false
// when the decomposition is unavailable or the corpus has fewer than
// minThemeParticipants participating issues — callers then show no themes. A nil
// cache, decomposition cache, or store yields (zero, false, nil).
//
// The whole call is guarded by the mutex, so a burst of first requests after a
// revision bump computes the factorization exactly once; the rest block and read
// the memoized result. The cached Result is immutable after compute, so
// concurrent readers are safe.
func (c *Cache) Current(ctx context.Context) (Result, bool, error) {
	if c == nil || c.Decomp == nil || c.Store == nil {
		return Result{}, false, nil
	}

	rev := c.currentRevision()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.computed && rev != 0 && rev == c.revision {
		return c.result, c.ok, nil
	}

	result, ok, err := c.compute(ctx, rev)
	if err != nil {
		return Result{}, false, err
	}

	c.result = result
	c.ok = ok
	c.revision = rev
	c.computed = true
	return result, ok, nil
}

func (c *Cache) compute(ctx context.Context, rev uint64) (Result, bool, error) {
	c.computeCount++

	decomp, ok, err := c.Decomp.Current(ctx)
	if err != nil {
		return Result{}, false, err
	}
	if !ok || decomp == nil || !decomp.Decomposed() {
		return Result{}, false, nil
	}

	items, err := c.Store.List(ctx)
	if err != nil {
		return Result{}, false, err
	}

	loadings, counts := loadingsFromDecomposition(decomp, items)
	if counts.participating < minThemeParticipants {
		return Result{}, false, nil
	}

	// Defaults: K = 8, 50 iterations, top-5 tags, issuethemes' own clamping.
	// TagEmbeddings from the bundle are corpus-mean centered, so theme centroids
	// land in the same centered space as issue embeddings — the space WP-204's
	// centroid-nearest lookup compares against.
	factorization := issuethemes.Build(loadings, decomp.TagNames, decomp.TagEmbeddings, issuethemes.Options{})
	return Result{
		Result:              factorization,
		Revision:            rev,
		Participating:       counts.participating,
		ExcludedNoEmbedding: counts.noEmbedding,
		ExcludedNoAnchor:    counts.noAnchor,
	}, true, nil
}

func (c *Cache) currentRevision() uint64 {
	if c == nil || c.Revisions == nil {
		return 0
	}
	return c.Revisions.Revision()
}
