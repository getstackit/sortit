package memories

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"sortit/internal/domain"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
	"sortit/internal/tags"
	"sortit/internal/vectors"
)

// Recall defaults. A query-time recall is an explicit ask, so the floor is
// permissive: results carry their similarity so the caller can judge relevance.
const (
	defaultRecallLimit = 5
	maxRecallLimit     = 50
	// maxGuaranteedConcepts caps how many subject-tag concepts a recall
	// force-surfaces for the query's dominant tags.
	maxGuaranteedConcepts = 3
	// guaranteedConceptTagFloor is the minimum query-tag relevance for that tag's
	// concept to be force-surfaced ahead of the cosine ranking.
	guaranteedConceptTagFloor = 0.4
)

// RecallOptions tunes a memory recall.
type RecallOptions struct {
	// Limit caps the number of memories returned. Defaults to defaultRecallLimit,
	// clamped to maxRecallLimit.
	Limit int
	// MinSimilarity drops results below this cosine similarity. Zero returns the
	// full top-N ranked set.
	MinSimilarity float64
	// IncludeSubjectConcepts force-surfaces the concept bound to each of the
	// query's dominant tags, ahead of the cosine ranking — a concept is the node
	// the work orbits, so working on tag X should always recall X's concept even
	// when its body doesn't rank on raw similarity. It costs a tag analysis of the
	// query (not just an embed), so it is opt-in: on for explicit recall, off for
	// the cheaper search-issues ride-along.
	IncludeSubjectConcepts bool
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
// acts. When opts.IncludeSubjectConcepts is set, the concepts bound to the
// query's dominant tags are force-surfaced ahead of the ranking. Requires an
// enricher; recall is unavailable without one.
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

	embedding, queryTags, err := s.recallQuerySignal(ctx, query, opts.IncludeSubjectConcepts)
	if err != nil {
		return nil, err
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

	if opts.IncludeSubjectConcepts {
		out = s.surfaceSubjectConcepts(ctx, out, embedding, queryTags, limit)
	}
	return out, nil
}

// recallQuerySignal returns the query embedding and, when concept surfacing is
// requested, the query's tag-relevance profile. A full tag analysis also yields
// the embedding, so we make a single call rather than embedding twice.
func (s *Service) recallQuerySignal(ctx context.Context, query string, withTags bool) ([]float64, []domain.TagRelevance, error) {
	if !withTags {
		embedding, err := s.enricher.EmbedText(ctx, query)
		if err != nil {
			return nil, nil, fmt.Errorf("embed recall query: %w", err)
		}
		return embedding, nil, nil
	}

	result, err := s.enricher.AnalyzeText(ctx, query, issueenrichment.AnalyzeTextOptions{
		CandidateMode:      tags.CandidateModeRetrievalShortlist,
		Verify:             false,
		SkipPriorDecisions: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("analyze recall query: %w", err)
	}
	embedding := issueenrichment.Float32VectorToFloat64(result.Analyzed.Embedding.Vector)
	return embedding, result.TagScores, nil
}

// surfaceSubjectConcepts pulls the active concept bound to each of the query's
// dominant tags to the front of the results — a concept is the node the work
// orbits, so it leads regardless of its raw cosine. A concept already in the
// cosine ranking is reordered to the front (keeping its real similarity) rather
// than duplicated; the merged list is re-capped to the limit so concepts win.
func (s *Service) surfaceSubjectConcepts(
	ctx context.Context,
	ranked []RecalledMemory,
	queryEmbedding []float64,
	queryTags []domain.TagRelevance,
	limit int,
) []RecalledMemory {
	dominant := append([]domain.TagRelevance(nil), queryTags...)
	sort.SliceStable(dominant, func(i, j int) bool {
		return dominant[i].Relevance > dominant[j].Relevance
	})

	rankedSim := make(map[string]float64, len(ranked))
	for _, r := range ranked {
		rankedSim[r.ID] = r.Similarity
	}

	concepts := make([]RecalledMemory, 0, maxGuaranteedConcepts)
	conceptIDs := make(map[string]bool)
	seenTag := make(map[string]bool)
	for _, ts := range dominant {
		if len(concepts) >= maxGuaranteedConcepts || ts.Relevance < guaranteedConceptTagFloor {
			break
		}
		tag := domain.NormalizeTagName(ts.Tag)
		if tag == "" || seenTag[tag] {
			continue
		}
		seenTag[tag] = true

		concept, err := s.store.GetActiveConceptBySubjectTag(ctx, tag)
		if errors.Is(err, issues.ErrMemoryNotFound) {
			continue
		}
		if err != nil {
			s.logger.WarnContext(ctx, "recall concept lookup failed", "tag", tag, "error", err)
			continue
		}
		if conceptIDs[concept.ID] {
			continue
		}
		conceptIDs[concept.ID] = true

		// Reuse the cosine similarity if the concept was already ranked; otherwise
		// compute it (the concept was outside the cosine top-N).
		sim, ok := rankedSim[concept.ID]
		if !ok && len(concept.Embedding) == len(queryEmbedding) && len(queryEmbedding) > 0 {
			sim = math.Round(vectors.CosineSimilarity(queryEmbedding, concept.Embedding)*1000) / 1000
		}
		concepts = append(concepts, RecalledMemory{Memory: concept, Similarity: sim})
	}

	if len(concepts) == 0 {
		return ranked
	}
	merged := concepts
	for _, r := range ranked {
		if conceptIDs[r.ID] {
			continue // already pulled to the front
		}
		merged = append(merged, r)
	}
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}
