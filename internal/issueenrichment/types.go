package enrichment

import (
	"context"
	"log/slog"
	"time"

	"sortit/internal/ai"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

// Centerer supplies the revision-cached corpus means so the verifier can measure
// tag alignment in the same centered space the factor/ridge model uses.
// *centering.Cache satisfies it.
type Centerer interface {
	Current(ctx context.Context) (issuemath.CorpusMeans, error)
}

type IssueEnricher struct {
	analyzer    *ai.Analyzer
	catalog     *tags.CatalogService
	logger      *slog.Logger
	exemplars   *ExemplarPool
	memoryStore issues.MemoryStore
	centering   Centerer
}

type AnalyzeTextOptions struct {
	PreferredTags []string
	CandidateMode tags.CandidateMode
	Verify        bool
	// SkipPriorDecisions disables injecting relevant memories as tagging
	// context. Set it when enriching a memory itself, so memory tagging is not
	// influenced by other memories.
	SkipPriorDecisions bool
}

type AnalyzeTextResult struct {
	Analyzed     ai.AnalyzedIssue
	CandidateSet tags.CandidateTaxonomy
	TagScores    []issues.TagRelevance
}

const issueEnrichmentTimeout = 20 * time.Second
const persistedIssueHintLimit = 5

// Memory context: how many relevant memories to surface to the tagger, and the
// minimum cosine similarity for one to be considered relevant.
const memoryContextLimit = 3
const memoryContextSimFloor = 0.5

func NewIssueEnricher(analyzer *ai.Analyzer, catalog *tags.CatalogService, logger *slog.Logger) *IssueEnricher {
	return &IssueEnricher{
		analyzer:  analyzer,
		catalog:   catalog,
		logger:    logger,
		exemplars: DefaultExemplarPool(),
	}
}

// SetExemplarPool replaces the default exemplar pool. Pass nil to disable
// few-shot examples.
func (s *IssueEnricher) SetExemplarPool(pool *ExemplarPool) {
	s.exemplars = pool
}

// UseMemoryContext enables surfacing relevant memories ("documented prior
// decisions") to the tagger during enrichment. Pass nil to disable.
func (s *IssueEnricher) UseMemoryContext(store issues.MemoryStore) {
	s.memoryStore = store
}

// UseCentering supplies corpus means so the verifier suppresses tags that are
// anti-aligned in centered space (where the factor model lives). Without it,
// alignment is raw — which anisotropy keeps positive, so suppression won't fire.
func (s *IssueEnricher) UseCentering(centerer Centerer) {
	s.centering = centerer
}
