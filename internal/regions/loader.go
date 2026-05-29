package regions

import (
	"context"
	"sync"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
)

// ListResult is the payload returned by the loader.
type ListResult struct {
	Regions []RegionWithMetrics   `json:"regions"`
	Window  domain.TimeWindow     `json:"window"`
	Orphans *domain.CorpusOrphans `json:"orphans,omitempty"`
}

// Loader serves region metrics, caching results by (kind, window,
// corpus revision). When no RevisionSource is wired, every call
// recomputes.
type Loader struct {
	Store         Store
	Tags          TagReader
	MapProjection MapProjectionReader
	CustomStore   CustomRegionStore
	Revisions     RevisionSource

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	revision uint64
	result   ListResult
}

// List returns all regions of the given kind for the given window. now
// is injectable for deterministic tests; production callers pass
// time.Now().UTC(). Unsupported kinds return an empty ListResult.
func (l *Loader) List(ctx context.Context, kind domain.RegionKind, window domain.TimeWindow, now time.Time) (ListResult, error) {
	if l == nil || l.Store == nil {
		return ListResult{Window: window}, nil
	}
	if kind == "" {
		kind = domain.RegionKindTag
	}

	rev := l.currentRevision()
	cacheKey := cacheKeyFor(kind, window.Label)

	l.mu.Lock()
	if entry, ok := l.cache[cacheKey]; ok && rev != 0 && entry.revision == rev {
		result := entry.result
		l.mu.Unlock()
		return result, nil
	}
	l.mu.Unlock()

	items, err := l.Store.List(ctx)
	if err != nil {
		return ListResult{}, err
	}
	events, err := l.listEventsForWindow(ctx, window)
	if err != nil {
		return ListResult{}, err
	}

	var regions []RegionWithMetrics
	var orphansPtr *domain.CorpusOrphans

	switch kind {
	case domain.RegionKindTag:
		regions = ListRegionsWithMetrics(items, events, window, now)
		tags, err := l.listTags(ctx)
		if err != nil {
			return ListResult{}, err
		}
		orphans := ComputeCorpusOrphans(items, tags)
		orphansPtr = &orphans
	case domain.RegionKindCluster:
		clusters, err := l.listClusters(ctx)
		if err != nil {
			return ListResult{}, err
		}
		regions = ListClusterRegionsWithMetrics(clusters, items, events, window, now)
	case domain.RegionKindCustom:
		customs, err := l.listCustomRegions(ctx)
		if err != nil {
			return ListResult{}, err
		}
		regions = ListCustomRegionsWithMetrics(customs, items, events, window, now)
	default:
		return ListResult{Window: window}, nil
	}

	result := ListResult{
		Regions: regions,
		Window:  window,
		Orphans: orphansPtr,
	}

	l.mu.Lock()
	if l.cache == nil {
		l.cache = make(map[string]cacheEntry)
	}
	l.cache[cacheKey] = cacheEntry{revision: rev, result: result}
	l.mu.Unlock()

	return result, nil
}

// Get returns one region's metrics by key. Reuses the cached list. The
// boolean is false when the region has no member issues.
func (l *Loader) Get(ctx context.Context, key domain.RegionKey, window domain.TimeWindow, now time.Time) (RegionWithMetrics, bool, error) {
	switch key.Kind {
	case domain.RegionKindTag:
		result, err := l.List(ctx, domain.RegionKindTag, window, now)
		if err != nil {
			return RegionWithMetrics{}, false, err
		}
		target := domain.NormalizeTagName(key.ID)
		for _, r := range result.Regions {
			if domain.NormalizeTagName(r.Region.Key.ID) == target {
				return r, true, nil
			}
		}
		return RegionWithMetrics{}, false, nil
	case domain.RegionKindCluster:
		result, err := l.List(ctx, domain.RegionKindCluster, window, now)
		if err != nil {
			return RegionWithMetrics{}, false, err
		}
		for _, r := range result.Regions {
			if r.Region.Key.ID == key.ID {
				return r, true, nil
			}
		}
		return RegionWithMetrics{}, false, nil
	case domain.RegionKindCustom:
		result, err := l.List(ctx, domain.RegionKindCustom, window, now)
		if err != nil {
			return RegionWithMetrics{}, false, err
		}
		for _, r := range result.Regions {
			if r.Region.Key.ID == key.ID {
				return r, true, nil
			}
		}
		return RegionWithMetrics{}, false, nil
	}
	return RegionWithMetrics{}, false, nil
}

func cacheKeyFor(kind domain.RegionKind, windowLabel string) string {
	return string(kind) + "|" + windowLabel
}

// Invalidate clears the cache. Wired into the issue event listener so
// changes to the corpus invalidate metrics on the next request.
func (l *Loader) Invalidate() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache = nil
}

func (l *Loader) currentRevision() uint64 {
	if l == nil || l.Revisions == nil {
		return 0
	}
	return l.Revisions.Revision()
}

// listEventsForWindow fetches the events that ComputeChurn needs. Returns
// nil for the "all" window, since flow metrics aren't defined without a
// finite span.
func (l *Loader) listEventsForWindow(ctx context.Context, window domain.TimeWindow) ([]issues.Event, error) {
	if window.Start.IsZero() {
		return nil, nil
	}
	return l.Store.ListLifecycleEvents(ctx, ChurnKinds, window.Start, window.End)
}

// listTags returns the catalog snapshot used for orphan detection. When
// no TagReader is wired (e.g., in unit tests), returns an empty slice so
// orphan computation degrades gracefully.
func (l *Loader) listTags(ctx context.Context) ([]issues.Tag, error) {
	if l.Tags == nil {
		return nil, nil
	}
	return l.Tags.StoredTags(ctx)
}

// listClusters returns the current k-means clusters from the cached map
// projection. When no MapProjectionReader is wired, returns nil so
// cluster-region requests degrade to an empty list.
func (l *Loader) listClusters(ctx context.Context) ([]issuemap.Cluster, error) {
	if l.MapProjection == nil {
		return nil, nil
	}
	projection, err := l.MapProjection.Current(ctx)
	if err != nil {
		return nil, err
	}
	return projection.Clusters, nil
}

// listCustomRegions returns the saved custom-region definitions. When
// no CustomStore is wired, returns nil so the kind degrades to an
// empty list.
func (l *Loader) listCustomRegions(ctx context.Context) ([]domain.CustomRegion, error) {
	if l.CustomStore == nil {
		return nil, nil
	}
	return l.CustomStore.ListCustomRegions(ctx)
}
