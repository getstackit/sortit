package regions

import (
	"context"

	"sortit/internal/issues"
)

// Store is the minimal read interface required to compute region metrics.
// Phase 1 walks the corpus on each compute pass; a projection-backed
// implementation can satisfy the same interface later.
type Store interface {
	List(ctx context.Context) ([]issues.Issue, error)
}

// RevisionSource is the optional capability for revision-based cache
// invalidation. When wired, the loader caches results until the revision
// changes.
type RevisionSource interface {
	Revision() uint64
}
