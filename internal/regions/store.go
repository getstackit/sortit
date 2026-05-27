package regions

import (
	"context"
	"time"

	"sortit/internal/issues"
)

// ChurnKinds lists the event kinds counted toward region churn:
// refinements (text edits) and the three issue-graph operations
// (split, combine, link). These match what `internal/issues/commands`
// records into the events log.
var ChurnKinds = []string{"refinement", "split", "combine", "link"}

// Store is the minimal read interface required to compute region metrics.
// Phase 1 walks the corpus on each compute pass; a projection-backed
// implementation can satisfy the same interface later.
type Store interface {
	List(ctx context.Context) ([]issues.Issue, error)
	ListLifecycleEvents(ctx context.Context, kinds []string, start, end time.Time) ([]issues.Event, error)
}

// RevisionSource is the optional capability for revision-based cache
// invalidation. When wired, the loader caches results until the revision
// changes.
type RevisionSource interface {
	Revision() uint64
}
