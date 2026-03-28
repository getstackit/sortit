package enrichment

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

type scoredExemplar struct {
	index               int
	similarity          float64
	sharedCount         int
	specificSharedCount int
}

const (
	exemplarBroadSharedSimilarityFloor = 0.42
	exemplarFallbackSimilarityFloor    = 0.55
)

var broadExemplarTags = map[string]struct{}{
	"backend":     {},
	"bug":         {},
	"cleanup":     {},
	"feature":     {},
	"frontend":    {},
	"idea":        {},
	"improvement": {},
	"performance": {},
	"ui":          {},
	"ux":          {},
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
		name = normalizeTagName(name)
		if name == "" {
			continue
		}
		candidateSet[name] = struct{}{}
	}

	candidates := make([]scoredExemplar, 0, len(p.items))
	for i, item := range p.items {
		if len(item.Embedding) == 0 {
			continue
		}
		sim := vectors.CosineSimilarity(issueEmbedding, item.Embedding)

		sharedCount := 0
		specificSharedCount := 0
		if len(candidateSet) > 0 {
			for _, tag := range item.Tags {
				name := normalizeTagName(tag.Name)
				if _, ok := candidateSet[name]; ok {
					sharedCount++
					if !isBroadExemplarTag(name) {
						specificSharedCount++
					}
				}
			}
		}

		candidates = append(candidates, scoredExemplar{
			index:               i,
			similarity:          sim,
			sharedCount:         sharedCount,
			specificSharedCount: specificSharedCount,
		})
	}

	slices.SortFunc(candidates, func(a, b scoredExemplar) int {
		if a.specificSharedCount != b.specificSharedCount {
			return cmp.Compare(b.specificSharedCount, a.specificSharedCount)
		}
		if a.sharedCount != b.sharedCount {
			return cmp.Compare(b.sharedCount, a.sharedCount)
		}
		return cmp.Compare(b.similarity, a.similarity)
	})

	out := make([]ai.FewShotExample, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		if !allowExemplarCandidate(candidate) {
			continue
		}
		out = append(out, p.items[candidate.index])
		if len(out) >= limit {
			break
		}
	}
	return out
}

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

func allowExemplarCandidate(candidate scoredExemplar) bool {
	if candidate.specificSharedCount > 0 {
		return true
	}
	if candidate.sharedCount > 0 {
		return candidate.similarity >= exemplarBroadSharedSimilarityFloor
	}
	return candidate.similarity >= exemplarFallbackSimilarityFloor
}

func isBroadExemplarTag(name string) bool {
	name = normalizeTagName(name)
	_, ok := broadExemplarTags[name]
	return ok
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
