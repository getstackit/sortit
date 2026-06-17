// Package ridgelambda memoizes the GCV-selected unscored-tag penalty for the
// anchored-ridge similarity model (see internal/issuemath/ridge_gcv.go),
// cached by corpus revision, following the same pattern as
// internal/centering and internal/tagcooccurrence.
//
// The penalty is a corpus-level property — it depends on the tag catalog's
// conditioning and the embedding dimension, not on any one query — so it is
// computed over the full corpus and reused across searches. Search handlers
// pass it to issuemap.WithRidgeSimilarity; the ranker itself never runs the
// selection, because the candidate set it sees may be a retrieved subset.
package ridgelambda

import (
	"context"
	"sort"
	"sync"

	"sortit/internal/centering"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/scoring"
)

// MaxGCVSampleIssues bounds how many issues feed the GCV grid search. The
// selected penalty is a smooth, coarse (grid-quantized) corpus property, so a
// deterministic stride sample estimates it just as well as the full corpus
// while keeping the per-revision recompute cheap (each candidate runs a
// per-issue Cholesky solve). Corpora at or below this size use every issue.
const MaxGCVSampleIssues = 2000

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

// Cache memoizes the GCV-selected unscored penalty by revision.
type Cache struct {
	Store     IssueLister
	Tags      TagSource
	Revisions RevisionSource
	// Centering supplies the revision-cached corpus means so the GCV solve
	// runs in the same centered space as the ranker. Optional — when nil the
	// embeddings are centered against their own freshly computed means.
	Centering *centering.Cache

	mu       sync.Mutex
	revision uint64
	lambda   float64
	ok       bool
	computed bool
}

// Current returns the GCV-selected unscored penalty at the current revision,
// recomputing it when the cache is empty or the revision has bumped. The
// boolean is false when the corpus is too small or degenerate to select a
// penalty — callers then fall back to the rank-1 similarity model. A nil
// cache or store yields (0, false).
func (c *Cache) Current(ctx context.Context) (float64, bool, error) {
	if c == nil || c.Store == nil {
		return 0, false, nil
	}

	rev := c.currentRevision()

	c.mu.Lock()
	if c.computed && rev != 0 && rev == c.revision {
		lambda, ok := c.lambda, c.ok
		c.mu.Unlock()
		return lambda, ok, nil
	}
	c.mu.Unlock()

	lambda, ok, err := c.compute(ctx)
	if err != nil {
		return 0, false, err
	}

	c.mu.Lock()
	c.lambda = lambda
	c.ok = ok
	c.revision = rev
	c.computed = true
	c.mu.Unlock()

	return lambda, ok, nil
}

func (c *Cache) compute(ctx context.Context) (float64, bool, error) {
	items, err := c.Store.List(ctx)
	if err != nil {
		return 0, false, err
	}
	var storeTags []issues.Tag
	if c.Tags != nil {
		if storeTags, err = c.Tags.StoredTags(ctx); err != nil {
			return 0, false, err
		}
	}
	if len(items) < scoring.MinDecompositionIssues || len(storeTags) == 0 {
		return 0, false, nil
	}

	tagNames := make([]string, 0, len(storeTags))
	rawTagEmbeddings := make(map[string][]float64, len(storeTags))
	for _, tag := range storeTags {
		if len(tag.Embedding) == 0 {
			continue
		}
		tagNames = append(tagNames, tag.Name)
		rawTagEmbeddings[tag.Name] = tag.Embedding
	}
	if len(tagNames) == 0 {
		return 0, false, nil
	}
	sort.Strings(tagNames)

	sample := strideSample(items, MaxGCVSampleIssues)
	rawIssueEmbeddings := make(map[string][]float64, len(sample))
	for _, item := range sample {
		if len(item.Embedding) == 0 {
			continue
		}
		rawIssueEmbeddings[item.ID] = item.Embedding
	}
	if len(rawIssueEmbeddings) < scoring.MinDecompositionIssues {
		return 0, false, nil
	}

	issueEmbeddings, tagEmbeddings, err := c.centeredEmbeddings(ctx, rawIssueEmbeddings, rawTagEmbeddings)
	if err != nil {
		return 0, false, err
	}

	lambda, ok := issuemath.SelectRidgeLambdaGCV(sample, tagNames, issueEmbeddings, tagEmbeddings,
		scoring.RidgeAnchorLambdaScored, nil)
	return lambda, ok, nil
}

// centeredEmbeddings centers the GCV inputs against the revision-cached corpus
// means when a centering cache is wired, matching the ranker's space; without
// one it centers against freshly computed means.
func (c *Cache) centeredEmbeddings(
	ctx context.Context,
	rawIssueEmbeddings, rawTagEmbeddings map[string][]float64,
) (map[string][]float64, map[string][]float64, error) {
	if c.Centering != nil {
		means, err := c.Centering.Current(ctx)
		if err != nil {
			return nil, nil, err
		}
		issueEmb, tagEmb := issuemath.CenterEmbeddingsWith(means, rawIssueEmbeddings, rawTagEmbeddings)
		return issueEmb, tagEmb, nil
	}
	issueEmb, tagEmb, _ := issuemath.CenterEmbeddings(rawIssueEmbeddings, rawTagEmbeddings)
	return issueEmb, tagEmb, nil
}

func (c *Cache) currentRevision() uint64 {
	if c == nil || c.Revisions == nil {
		return 0
	}
	return c.Revisions.Revision()
}

// strideSample returns at most max issues drawn at an even stride over the
// ID-sorted corpus, so the sample is deterministic and spread across the
// corpus rather than biased to one end.
func strideSample(items []issues.Issue, max int) []issues.Issue {
	if len(items) <= max {
		return items
	}
	sorted := append([]issues.Issue(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	out := make([]issues.Issue, 0, max)
	// Fixed-point stride so the indices span [0, len) deterministically.
	step := float64(len(sorted)) / float64(max)
	for i := range max {
		out = append(out, sorted[int(float64(i)*step)])
	}
	return out
}
