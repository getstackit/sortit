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

	"sortit/internal/issuemath"
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
	// Means are the corpus-mean centering vectors the decomposition (and this
	// factorization's TagEmbeddings/Centroid space) were computed in. WP-204's
	// centroid-nearest-issue lookup needs to center a raw issue embedding into
	// the same space as a theme's Centroid; the decomposition cache already
	// computes these means (internal/ridgedecomp reads them from
	// internal/centering), so this field just carries the value through rather
	// than re-deriving it. Zero when the decomposition centered against
	// nothing (e.g. no centering cache wired) — issuemath.CenterVector
	// degrades to unit-normalization in that case.
	Means issuemath.CorpusMeans

	// Identities carries the stable theme identity metadata, index-aligned with
	// the embedded Result.Themes: Identities[i] is the stable ID and match score
	// for Themes[i]. Populated by WP-203 identity matching against the previous
	// revision's themes retained in the cache.
	Identities []ThemeIdentity
	// MintedIDs are the stable IDs first assigned this revision — themes with no
	// qualifying predecessor (a fresh theme, a split-off, or a match that fell
	// below themeMatchThreshold). WP-206 aggregates these as churn telemetry.
	MintedIDs []string
	// RetiredIDs are previous-revision stable IDs that no new theme inherited —
	// a theme that disappeared or merged into another. Consumers decide whether
	// to keep showing a retired theme; the cache stops tracking it. WP-206
	// telemetry input.
	RetiredIDs []string

	// Labels carries the WP-205 human name/description for each theme,
	// index-aligned with Themes (Labels[i] labels Themes[i]) like Identities.
	// A newly minted or relabeled theme carries the deterministic top-tag
	// fallback (e.g. "mobile / ui") immediately; when a Labeler is configured the
	// LLM name swaps in asynchronously (never blocking compute — see the Cache
	// doc). Empty when no theme has a stable ID.
	Labels []ThemeLabel
}

// Cache memoizes the corpus theme factorization by revision and carries theme
// identity across those revisions.
//
// Identity (WP-203): the cache retains the previous revision's themes (H rows by
// stable ID) so a recompute can match new themes to old ones and keep stable IDs
// steady through small corpus mutations — NMF alone guarantees only that the
// SAME input is reproducible, not that SIMILAR inputs stay aligned. This identity
// state is in-memory, so constructing a new Cache (a process restart) resets it:
// the first compute after restart mints fresh IDs for every theme. That is
// acceptable at the debug tier (WP-204); persisting the tiny identity state is
// the second trigger for theme persistence (after WP-405's drift snapshots) and
// is deferred to Stage 4, when user-visible profiles make a restart-time ID
// change a visible regression.
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

	// Labeler names and describes themes via an LLM. Optional — when nil every
	// theme keeps its deterministic top-tag fallback label and no background work
	// is spawned. When set, minted/relabeled themes are labeled asynchronously so
	// labeling never blocks or delays Current (see labeling.go).
	Labeler Labeler

	mu           sync.Mutex
	revision     uint64
	result       Result
	ok           bool
	computed     bool
	computeCount int // number of actual computes; asserted by the concurrency test

	// identity is the cross-revision theme identity memory (previous H rows by
	// stable ID + the mint counter). Retained across recomputes so stable IDs
	// survive small corpus mutations; reset only by constructing a new Cache
	// (process restart). Mutated only inside compute, under mu.
	identity identityState

	// labels is the in-memory theme-label store keyed by stable theme ID (see
	// labeling.go). Like identity it is in-memory only, so a process restart
	// relabels — acceptable and documented at the debug tier; the durable
	// theme_labels table lands with Stage-4 identity persistence. Mutated under mu
	// (from compute, and from the async label writeback).
	labels themeLabelStore
	// labelInFlight guards against launching a duplicate labeling job for a stable
	// ID whose current label generation is already being computed; keyed by ID,
	// valued by the label generation in flight. Guarded by mu.
	labelInFlight map[string]int
	// labelWG tracks outstanding async label jobs so tests can wait for them (and
	// prove no goroutine leaks); production never waits.
	labelWG sync.WaitGroup
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
	result := Result{
		Result:              factorization,
		Revision:            rev,
		Participating:       counts.participating,
		ExcludedNoEmbedding: counts.noEmbedding,
		ExcludedNoAnchor:    counts.noAnchor,
		Means:               decomp.Means,
	}
	c.identify(&result)
	c.label(&result, items)
	return result, true, nil
}

// identify attaches stable theme IDs and mint/retire diagnostics to a freshly
// factorized result by matching its themes against the previous revision's, then
// advances the cross-revision identity state to this revision. It runs only on a
// successful (ok) compute — a degenerate revision that returns (zero, false)
// leaves the identity state untouched, so identity resumes (rather than churns)
// if the corpus recovers above the participation floor. Called only from
// compute, under c.mu.
func (c *Cache) identify(result *Result) {
	newRows := make([]themeRow, len(result.Themes))
	for i, theme := range result.Themes {
		newRows[i] = themeRowFrom(theme)
	}

	match := matchThemes(c.identity.prev, newRows, c.identity.counter)

	identities := make([]ThemeIdentity, len(newRows))
	next := make([]identifiedTheme, len(newRows))
	for i := range newRows {
		identities[i] = ThemeIdentity{StableID: match.ids[i], MatchScore: match.scores[i]}
		next[i] = identifiedTheme{id: match.ids[i], row: newRows[i]}
	}
	result.Identities = identities
	result.MintedIDs = match.minted
	result.RetiredIDs = match.retired

	c.identity.prev = next
	c.identity.counter = match.counter
}

func (c *Cache) currentRevision() uint64 {
	if c == nil || c.Revisions == nil {
		return 0
	}
	return c.Revisions.Revision()
}
