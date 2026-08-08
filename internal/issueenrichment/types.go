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
	// Trace records the decisions made at every boundary of text enrichment.
	// It intentionally contains counts and derived metadata rather than prompt
	// bodies or retrieved-memory text, so it is safe to expose through the
	// authenticated debug API. It is the unit a fixture evaluator can snapshot
	// and compare as the pipeline evolves.
	Trace AnalysisTrace
}

// AnalysisTrace is an inspectable, JSON-friendly record of a single text
// enrichment run. The pipeline still returns its normal result; the trace adds
// observability without making callers reconstruct intermediate state from log
// lines or AI responses.
type AnalysisTrace struct {
	Input              AnalysisTraceInput              `json:"input"`
	CandidateSelection AnalysisTraceCandidateSelection `json:"candidateSelection"`
	Context            AnalysisTraceContext            `json:"context"`
	ModelOutput        AnalysisTraceModelOutput        `json:"modelOutput"`
	PostProcessing     AnalysisTracePostProcessing     `json:"postProcessing"`
	Verification       AnalysisTraceVerification       `json:"verification"`
}

type AnalysisTraceInput struct {
	CharacterCount int `json:"characterCount"`
}

type AnalysisTraceCandidateSelection struct {
	Mode           tags.CandidateMode `json:"mode"`
	CandidateCount int                `json:"candidateCount"`
	HintCount      int                `json:"hintCount"`
	SourceCounts   map[string]int     `json:"sourceCounts"`
}

type AnalysisTraceContext struct {
	FewShotExampleCount int  `json:"fewShotExampleCount"`
	PriorDecisionCount  int  `json:"priorDecisionCount"`
	ConceptCount        int  `json:"conceptCount"`
	HasProjectOverview  bool `json:"hasProjectOverview"`
}

type AnalysisTraceModelOutput struct {
	AssignedTagCount int `json:"assignedTagCount"`
	NegatedTagCount  int `json:"negatedTagCount"`
	// Tags and Negated are the model response before relevance-floor filtering,
	// generic attenuation, evidence resolution, and deterministic verification.
	// Comparing these with AnalyzeTextResult.TagScores isolates model behavior
	// from deterministic post-processing in an evaluator.
	Tags    []ai.TagScore   `json:"tags"`
	Negated []ai.NegatedTag `json:"negated"`
}

type AnalysisTracePostProcessing struct {
	RelevanceFloorFilteredCount int      `json:"relevanceFloorFilteredCount"`
	GenericAttenuatedTags       []string `json:"genericAttenuatedTags"`
	PersistedTagCount           int      `json:"persistedTagCount"`
}

type AnalysisTraceVerification struct {
	Enabled         bool `json:"enabled"`
	KeepCount       int  `json:"keepCount"`
	DownRankedCount int  `json:"downRankedCount"`
	FlaggedCount    int  `json:"flaggedCount"`
	NegatedCount    int  `json:"negatedCount"`
	EvidenceCount   int  `json:"evidenceCount"`
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
