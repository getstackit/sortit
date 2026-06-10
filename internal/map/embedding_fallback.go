package issuemap

import (
	"log/slog"
	"maps"
	"sync"
	"time"
)

// Embedding fallback kinds. whitepaper.md §9 calls the hash-based
// pseudo-embedding a soft correctness risk: it should never stand in for a
// persisted embedding on a healthy install, and when it does, quality
// degrades invisibly because the system keeps producing results. Each
// fallback decision is therefore counted by the kind of embedding that was
// missing, and the counts are exposed via
// GET /api/v1/debug/embedding-fallbacks.
const (
	EmbeddingFallbackKindIssue = "issue"
	EmbeddingFallbackKindTag   = "tag"
)

// embeddingFallbackLogInterval rate-limits the warning log per kind so a
// corpus full of missing embeddings doesn't flood the logs — the counter
// keeps the precise tally.
const embeddingFallbackLogInterval = time.Minute

type embeddingFallbackTracker struct {
	mu      sync.Mutex
	counts  map[string]uint64
	lastLog map[string]time.Time
}

var embeddingFallbacks = &embeddingFallbackTracker{
	counts:  make(map[string]uint64),
	lastLog: make(map[string]time.Time),
}

// recordEmbeddingFallback notes that a hash-based pseudo-embedding stood in
// for a missing persisted embedding. It only observes — the fallback
// behavior itself is unchanged.
func recordEmbeddingFallback(kind, id string) {
	embeddingFallbacks.record(kind, id, time.Now())
}

func (t *embeddingFallbackTracker) record(kind, id string, now time.Time) {
	t.mu.Lock()
	t.counts[kind]++
	count := t.counts[kind]
	shouldLog := now.Sub(t.lastLog[kind]) >= embeddingFallbackLogInterval
	if shouldLog {
		t.lastLog[kind] = now
	}
	t.mu.Unlock()

	if shouldLog {
		slog.Warn(
			"hash-embedding fallback fired: persisted embedding missing",
			"kind", kind,
			"id", id,
			"count", count,
		)
	}
}

func (t *embeddingFallbackTracker) snapshot() map[string]uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]uint64, len(t.counts))
	maps.Copy(out, t.counts)
	return out
}

// EmbeddingFallbackCounts returns a snapshot of how many times the
// hash-embedding fallback has fired since process start, keyed by kind
// (EmbeddingFallbackKindIssue, EmbeddingFallbackKindTag).
func EmbeddingFallbackCounts() map[string]uint64 {
	return embeddingFallbacks.snapshot()
}
