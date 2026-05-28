package regions

import (
	"context"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
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

// TagReader supplies the catalog snapshot used by ComputeCorpusOrphans
// to find each issue's nearest tag in embedding space. The CatalogService
// satisfies this interface via its StoredTags method.
type TagReader interface {
	StoredTags(ctx context.Context) ([]issues.Tag, error)
}

// MapProjectionReader supplies the current k-means clusters used by
// cluster-region compute. *mapview.MapProjectionLoader satisfies this
// interface via its Current method.
type MapProjectionReader interface {
	Current(ctx context.Context) (issuemap.MapProjection, error)
}

// CustomRegionStore is the read interface the loader uses to fetch
// user-defined region definitions. issues.PostgresStore and
// issues.InMemoryStore both satisfy it.
type CustomRegionStore interface {
	ListCustomRegions(ctx context.Context) ([]domain.CustomRegion, error)
	GetCustomRegion(ctx context.Context, id string) (domain.CustomRegion, error)
}

// RevisionSource is the optional capability for revision-based cache
// invalidation. When wired, the loader caches results until the revision
// changes.
type RevisionSource interface {
	Revision() uint64
}
