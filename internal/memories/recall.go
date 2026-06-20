package memories

import (
	"context"
	"fmt"
	"strings"

	"sortit/internal/domain"
)

// Recall defaults. A query-time recall is an explicit ask, so the floor is
// permissive: results carry their similarity so the caller can judge relevance.
const (
	defaultRecallLimit = 5
	maxRecallLimit     = 50
)

// RecallOptions tunes a memory recall.
type RecallOptions struct {
	// Limit caps the number of memories returned. Defaults to defaultRecallLimit,
	// clamped to maxRecallLimit.
	Limit int
	// MinSimilarity drops results below this cosine similarity. Zero returns the
	// full top-N ranked set.
	MinSimilarity float64
}

// RecalledMemory is a memory plus its cosine similarity to the recall query.
// It embeds domain.Memory so the JSON shape matches list/get plus a similarity.
type RecalledMemory struct {
	domain.Memory
	Similarity float64 `json:"similarity"`
}

// RecallMemories returns the active memories most similar to the query text,
// nearest first. It embeds the query (reusing the enrichment embedder) and
// ranks active memories by cosine similarity — the read side of the memory
// loop, so prior decisions, constraints, and patterns surface before an agent
// acts. Requires an enricher; recall is unavailable without one.
func (s *Service) RecallMemories(ctx context.Context, query string, opts RecallOptions) ([]RecalledMemory, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("recall query is required")
	}
	if s.enricher == nil {
		return nil, fmt.Errorf("memory recall requires an analyzer")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultRecallLimit
	}
	if limit > maxRecallLimit {
		limit = maxRecallLimit
	}

	embedding, err := s.enricher.EmbedText(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed recall query: %w", err)
	}

	results, err := s.store.SearchMemories(ctx, embedding, limit)
	if err != nil {
		return nil, fmt.Errorf("recall memories: %w", err)
	}

	out := make([]RecalledMemory, 0, len(results))
	for _, result := range results {
		if result.Similarity < opts.MinSimilarity {
			continue
		}
		out = append(out, RecalledMemory{Memory: result.Memory, Similarity: result.Similarity})
	}
	return out, nil
}
