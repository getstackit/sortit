package regions

import (
	"context"
	"sync"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

// ListResult is the payload returned by the loader.
type ListResult struct {
	Regions []RegionWithMetrics   `json:"regions"`
	Window  domain.TimeWindow     `json:"window"`
	Orphans *domain.CorpusOrphans `json:"orphans,omitempty"`
}

// Loader serves region metrics, caching results by (window, corpus
// revision). When no RevisionSource is wired, every call recomputes.
type Loader struct {
	Store     Store
	Tags      TagReader
	Revisions RevisionSource

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	revision uint64
	result   ListResult
}

// List returns all regions for the given window. The now parameter lets
// tests inject a deterministic clock; production callers pass
// time.Now().UTC().
func (l *Loader) List(ctx context.Context, window domain.TimeWindow, now time.Time) (ListResult, error) {
	if l == nil || l.Store == nil {
		return ListResult{Window: window}, nil
	}

	rev := l.currentRevision()

	l.mu.Lock()
	if entry, ok := l.cache[window.Label]; ok && rev != 0 && entry.revision == rev {
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
	tags, err := l.listTags(ctx)
	if err != nil {
		return ListResult{}, err
	}
	orphans := ComputeCorpusOrphans(items, tags)
	result := ListResult{
		Regions: ListRegionsWithMetrics(items, events, window, now),
		Window:  window,
		Orphans: &orphans,
	}

	l.mu.Lock()
	if l.cache == nil {
		l.cache = make(map[string]cacheEntry)
	}
	l.cache[window.Label] = cacheEntry{revision: rev, result: result}
	l.mu.Unlock()

	return result, nil
}

// Get returns one region's metrics by key. Reuses the cached list. The
// boolean is false when the region has no member issues.
func (l *Loader) Get(ctx context.Context, key domain.RegionKey, window domain.TimeWindow, now time.Time) (RegionWithMetrics, bool, error) {
	if key.Kind != domain.RegionKindTag {
		return RegionWithMetrics{}, false, nil
	}
	result, err := l.List(ctx, window, now)
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
