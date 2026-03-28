package enrichment

import (
	"log/slog"
	"time"

	"splat/internal/ai"
	"splat/internal/issues"
	"splat/internal/services"
)

type IssueEnricher struct {
	analyzer  *ai.Analyzer
	catalog   *services.CatalogService
	logger    *slog.Logger
	exemplars *ExemplarPool
}

type AnalyzeTextOptions struct {
	PreferredTags []string
	CandidateMode services.CandidateMode
	Verify        bool
}

type AnalyzeTextResult struct {
	Analyzed     ai.AnalyzedIssue
	CandidateSet services.CandidateTaxonomy
	TagScores    []issues.TagRelevance
}

const issueEnrichmentTimeout = 20 * time.Second
const persistedIssueHintLimit = 5

func NewIssueEnricher(analyzer *ai.Analyzer, catalog *services.CatalogService, logger *slog.Logger) *IssueEnricher {
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
