package services

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"splat/internal/ai"
	"splat/internal/vectors"
)

// ExemplarPool holds curated tagging examples and lazily computes their
// embeddings for similarity-based selection.
type ExemplarPool struct {
	items []ai.FewShotExample

	mu       sync.Mutex
	embedded bool
}

// NewExemplarPool creates a pool from a curated list of examples.
func NewExemplarPool(items []ai.FewShotExample) *ExemplarPool {
	if len(items) == 0 {
		return nil
	}
	pool := make([]ai.FewShotExample, len(items))
	copy(pool, items)
	return &ExemplarPool{items: pool}
}

// textEmbedder is the minimal interface needed to compute exemplar embeddings.
type textEmbedder interface {
	EmbedText(ctx context.Context, text string) (ai.EmbeddingResult, error)
}

// Select returns up to limit exemplars most similar to issueEmbedding.
// If embeddings have not been computed yet, it computes them lazily using the
// provided embedder. If embedding fails for an exemplar it is silently skipped.
// candidateTags, when non-empty, is used to prefer examples that share at least
// one tag with the current candidate set.
func (p *ExemplarPool) Select(ctx context.Context, embedder textEmbedder, issueEmbedding []float64, candidateTags []string, limit int) []ai.FewShotExample {
	if p == nil || len(p.items) == 0 || len(issueEmbedding) == 0 || limit <= 0 {
		return nil
	}

	p.ensureEmbeddings(ctx, embedder)

	candidateSet := make(map[string]struct{}, len(candidateTags))
	for _, name := range candidateTags {
		candidateSet[name] = struct{}{}
	}

	type scored struct {
		index      int
		similarity float64
		shared     bool // shares a tag with the candidate set
	}

	candidates := make([]scored, 0, len(p.items))
	for i, item := range p.items {
		if len(item.Embedding) == 0 {
			continue
		}
		sim := vectors.CosineSimilarity(issueEmbedding, item.Embedding)

		shared := false
		if len(candidateSet) > 0 {
			for _, tag := range item.Tags {
				if _, ok := candidateSet[tag.Name]; ok {
					shared = true
					break
				}
			}
		}

		candidates = append(candidates, scored{
			index:      i,
			similarity: sim,
			shared:     shared,
		})
	}

	// Sort: prefer shared-tag examples, then by similarity descending.
	slices.SortFunc(candidates, func(a, b scored) int {
		if a.shared != b.shared {
			if a.shared {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.similarity, a.similarity)
	})

	n := min(limit, len(candidates))
	out := make([]ai.FewShotExample, n)
	for i := 0; i < n; i++ {
		out[i] = p.items[candidates[i].index]
	}
	return out
}

// ensureEmbeddings lazily computes embeddings for all exemplars.
func (p *ExemplarPool) ensureEmbeddings(ctx context.Context, embedder textEmbedder) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.embedded || embedder == nil {
		return
	}
	p.embedded = true

	for i := range p.items {
		if len(p.items[i].Embedding) > 0 {
			continue
		}
		result, err := embedder.EmbedText(ctx, p.items[i].Text)
		if err != nil {
			continue
		}
		p.items[i].Embedding = Float32VectorToFloat64(result.Vector)
	}
}

// DefaultExemplarPool returns the built-in curated exemplar pool.
func DefaultExemplarPool() *ExemplarPool {
	return NewExemplarPool([]ai.FewShotExample{
		{
			Text: "when searching for open issues on the open issues page i can only type in one letter into the search box, the second character i type clears the search box and the search.",
			Tags: []ai.FewShotTag{
				{Name: "bug", Relevance: 0.95},
				{Name: "frontend", Relevance: 0.85},
				{Name: "search", Relevance: 0.90},
			},
		},
		{
			Text: "CosineSimilarity is implemented 3 times across the codebase. internal/queries/vectors.go has a full cosine similarity, internal/map/similarity.go has an identical cosineSimilarity plus a unitCosineSimilarity variant. Extract to a shared internal/vectors or internal/math package.",
			Tags: []ai.FewShotTag{
				{Name: "backend", Relevance: 0.80},
				{Name: "code-duplication", Relevance: 0.92},
				{Name: "improvement", Relevance: 0.75},
			},
		},
		{
			Text: "Schema drift: api_tokens table in schema.sql is missing the name column that was added in migration 000013",
			Tags: []ai.FewShotTag{
				{Name: "bug", Relevance: 0.90},
				{Name: "database", Relevance: 0.95},
			},
		},
		{
			Text: "Tag map: encode specificity as node size in SVG projection",
			Tags: []ai.FewShotTag{
				{Name: "frontend", Relevance: 0.75},
				{Name: "map-page", Relevance: 0.90},
				{Name: "tagging-specificity", Relevance: 0.85},
			},
		},
		{
			Text: "Error wrapping is inconsistent across the backend. Some errors use fmt.Errorf with %w (wrapping), others use %v or errors.New, losing the error chain. Standardize on %w for all wrapped errors so callers can inspect error causes.",
			Tags: []ai.FewShotTag{
				{Name: "backend", Relevance: 0.90},
				{Name: "bug", Relevance: 0.80},
				{Name: "improvement", Relevance: 0.70},
			},
		},
	})
}
