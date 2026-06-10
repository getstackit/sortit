package diagnostics

import (
	"context"

	issuemap "sortit/internal/map"
)

// DebugEmbeddingFallbacksResult is the response body for
// GET /api/v1/debug/embedding-fallbacks. Counts are cumulative since
// process start. Non-zero values mean the hash-based pseudo-embedding
// (whitepaper.md §9) stood in for missing persisted embeddings — a signal
// that search and map quality are silently degraded.
type DebugEmbeddingFallbacksResult struct {
	Issue uint64 `json:"issue"`
	Tag   uint64 `json:"tag"`
	Total uint64 `json:"total"`
}

// DebugEmbeddingFallbacksHandler reports the in-process hash-embedding
// fallback counters tracked by internal/map.
type DebugEmbeddingFallbacksHandler struct{}

func (DebugEmbeddingFallbacksHandler) Handle(_ context.Context) (DebugEmbeddingFallbacksResult, error) {
	counts := issuemap.EmbeddingFallbackCounts()
	return DebugEmbeddingFallbacksResult{
		Issue: counts[issuemap.EmbeddingFallbackKindIssue],
		Tag:   counts[issuemap.EmbeddingFallbackKindTag],
		Total: counts[issuemap.EmbeddingFallbackKindIssue] + counts[issuemap.EmbeddingFallbackKindTag],
	}, nil
}
