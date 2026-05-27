package regions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sortit/internal/domain"
)

// ErrUnsupportedKind is returned when the request asks for a region kind
// that's reserved but not yet implemented (cluster, custom).
var ErrUnsupportedKind = errors.New("unsupported region kind")

// ErrRegionNotFound is returned when no region with the given key has
// any member issues.
var ErrRegionNotFound = errors.New("region not found")

// ListResponse is the payload for the list endpoint.
type ListResponse struct {
	Regions []RegionWithMetrics   `json:"regions"`
	Window  domain.TimeWindow     `json:"window"`
	Orphans *domain.CorpusOrphans `json:"orphans,omitempty"`
}

// GetResponse is the payload for the single-region endpoint.
type GetResponse struct {
	Region  domain.Region        `json:"region"`
	Metrics domain.RegionMetrics `json:"metrics"`
	Window  domain.TimeWindow    `json:"window"`
}

// Handler wraps the loader with window parsing and kind validation so the
// API layer stays thin and can focus on HTTP plumbing.
type Handler struct {
	Loader *Loader
	// Clock is injectable for tests. nil falls back to time.Now().UTC().
	Clock func() time.Time
}

// List returns regions for the given kind and window label.
func (h *Handler) List(ctx context.Context, kind, windowLabel string) (ListResponse, error) {
	if kind == "" {
		kind = string(domain.RegionKindTag)
	}
	regionKind, err := parseSupportedKind(kind)
	if err != nil {
		return ListResponse{}, err
	}
	if windowLabel == "" {
		windowLabel = DefaultWindowLabel
	}
	now := h.now()
	window, err := ParseTimeWindow(windowLabel, now)
	if err != nil {
		return ListResponse{}, err
	}
	result, err := h.Loader.List(ctx, regionKind, window, now)
	if err != nil {
		return ListResponse{}, err
	}
	return ListResponse(result), nil
}

// Get returns a single region's metrics, or ErrRegionNotFound if no
// issues are members.
func (h *Handler) Get(ctx context.Context, kind, id, windowLabel string) (GetResponse, error) {
	regionKind, err := parseSupportedKind(kind)
	if err != nil {
		return GetResponse{}, err
	}
	if windowLabel == "" {
		windowLabel = DefaultWindowLabel
	}
	now := h.now()
	window, err := ParseTimeWindow(windowLabel, now)
	if err != nil {
		return GetResponse{}, err
	}
	key := domain.RegionKey{Kind: regionKind, ID: id}
	region, ok, err := h.Loader.Get(ctx, key, window, now)
	if err != nil {
		return GetResponse{}, err
	}
	if !ok {
		return GetResponse{Window: window}, ErrRegionNotFound
	}
	return GetResponse{
		Region:  region.Region,
		Metrics: region.Metrics,
		Window:  window,
	}, nil
}

// parseSupportedKind validates the wire-form kind against the kinds the
// loader knows how to compute (tag and cluster). Returns
// ErrUnsupportedKind otherwise.
func parseSupportedKind(raw string) (domain.RegionKind, error) {
	switch raw {
	case string(domain.RegionKindTag):
		return domain.RegionKindTag, nil
	case string(domain.RegionKindCluster):
		return domain.RegionKindCluster, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnsupportedKind, raw)
}

// Orphans returns the corpus-level orphan signal: count and fraction of
// issues that belong to no region and have no nearby tag in embedding
// space. Window is ignored (orphan-ness is a snapshot quantity).
func (h *Handler) Orphans(ctx context.Context) (domain.CorpusOrphans, error) {
	now := h.now()
	// Reuse the 30d cached compute path so we don't re-walk issues here.
	window, err := ParseTimeWindow(DefaultWindowLabel, now)
	if err != nil {
		return domain.CorpusOrphans{}, err
	}
	result, err := h.Loader.List(ctx, domain.RegionKindTag, window, now)
	if err != nil {
		return domain.CorpusOrphans{}, err
	}
	if result.Orphans == nil {
		return domain.CorpusOrphans{}, nil
	}
	return *result.Orphans, nil
}

func (h *Handler) now() time.Time {
	if h.Clock != nil {
		return h.Clock()
	}
	return time.Now().UTC()
}
