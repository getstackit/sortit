package enrichment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"sortit/internal/ai"
	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

type analysisContext struct {
	fewShotExampleCount int
	priorDecisionCount  int
	conceptCount        int
	hasProjectOverview  bool
}

func (s *IssueEnricher) analyzeWithCandidateTaxonomy(ctx context.Context, raw string, preferred []string, mode tags.CandidateMode, includePriorDecisions bool) (ai.AnalyzedIssue, tags.CandidateTaxonomy, analysisContext, error) {
	freshEmbedding, err := s.analyzer.EmbedText(ctx, raw)
	if err != nil {
		return ai.AnalyzedIssue{}, tags.CandidateTaxonomy{}, analysisContext{}, fmt.Errorf("embed raw for shortlist: %w", err)
	}
	freshEmbeddingVector := Float32VectorToFloat64(freshEmbedding.Vector)

	candidates, err := s.catalog.IssueTaxonomyCandidates(ctx, freshEmbeddingVector, preferred, mode, 15)
	if err != nil {
		return ai.AnalyzedIssue{}, tags.CandidateTaxonomy{}, analysisContext{}, err
	}
	candidates = s.catalog.AnnotateCandidateHints(ctx, candidates, freshEmbeddingVector, persistedIssueHintLimit)

	var examples []ai.FewShotExample
	if s.exemplars != nil && s.analyzer != nil {
		candidateTagNames := make([]string, 0, len(candidates.Tags))
		for _, tag := range candidates.Tags {
			candidateTagNames = append(candidateTagNames, tag.Name)
		}
		examples = s.exemplars.Select(ctx, s.analyzer, freshEmbeddingVector, candidateTagNames, 3)
	}

	// The project concept frame and prior decisions both ground tagging in the
	// project's own knowledge, so they ride the same gate: a memory's own
	// enrichment runs clean (SkipPriorDecisions) and must not be influenced by
	// other memories' content — and the frame's concept profiles are exactly that.
	var priorDecisions []ai.PriorDecision
	var frame ai.ConceptFrame
	if includePriorDecisions {
		priorDecisions = s.relevantPriorDecisions(ctx, freshEmbeddingVector)
		frame = s.projectConceptFrame(ctx)
	}
	traceContext := analysisContext{
		fewShotExampleCount: len(examples),
		priorDecisionCount:  len(priorDecisions),
		conceptCount:        len(frame.Concepts),
		hasProjectOverview:  strings.TrimSpace(frame.Overview) != "",
	}

	analyzed, err := s.analyzer.AnalyzeIssueDataWithEmbedding(ctx, raw, candidates.AITags(), examples, frame, freshEmbedding, priorDecisions...)
	if err != nil {
		return ai.AnalyzedIssue{}, tags.CandidateTaxonomy{}, analysisContext{}, err
	}
	return analyzed, candidates, traceContext, nil
}

// EmbedText returns the float64 embedding for raw text — the same embedding the
// enrichment pipeline computes for shortlisting and memory context. It lets
// callers (e.g. query-time memory recall) reuse the configured embedder without
// running a full tag analysis.
func (s *IssueEnricher) EmbedText(ctx context.Context, raw string) ([]float64, error) {
	if s.analyzer == nil {
		return nil, fmt.Errorf("embed text: analyzer not configured")
	}
	result, err := s.analyzer.EmbedText(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("embed text: %w", err)
	}
	return Float32VectorToFloat64(result.Vector), nil
}

// relevantPriorDecisions retrieves the memories most similar to the text being
// enriched and shapes them as tagging context, so a new issue restating a
// settled decision is tagged consistently with it. Failures are non-fatal.
func (s *IssueEnricher) relevantPriorDecisions(ctx context.Context, embedding []float64) []ai.PriorDecision {
	if s.memoryStore == nil || len(embedding) == 0 {
		return nil
	}
	results, err := s.memoryStore.SearchMemories(ctx, embedding, memoryContextLimit)
	if err != nil {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "memory context retrieval failed", "error", err)
		}
		return nil
	}
	decisions := make([]ai.PriorDecision, 0, len(results))
	for _, result := range results {
		if result.Similarity < memoryContextSimFloor {
			continue
		}
		decisions = append(decisions, ai.PriorDecision{
			Title:   priorDecisionTitle(result.Memory),
			Summary: result.Memory.Body,
			Tags:    result.Memory.AnchorTags,
		})
	}
	return decisions
}

// conceptFrameMaxConcepts caps how many concept digests enter the tagging frame,
// bounding the stable prompt prefix on projects with a large concept set.
const conceptFrameMaxConcepts = 25

// conceptProfileMaxChars bounds each concept's profile in the frame so a long
// concept body doesn't dominate the prompt.
const conceptProfileMaxChars = 280

// projectConceptFrame assembles the project's identity frame from the memory
// store the enricher already holds. See ProjectConceptFrame for the assembly
// rules.
func (s *IssueEnricher) projectConceptFrame(ctx context.Context) ai.ConceptFrame {
	return ProjectConceptFrame(ctx, s.memoryStore, s.logger)
}

// ProjectConceptFrame assembles the project's identity frame for grounding issue
// tagging: the active overview paragraph plus a digest of the active concepts
// (the project's own vocabulary), ranked by reinforcement then recency and
// capped. It reads only the given memory store; without a store, or with neither
// an overview nor concepts set, it returns an empty frame (a no-op that renders
// the pre-frame prompt). Store failures are non-fatal. It is exported so the
// memory synthesizer can prime cluster-concept naming with the same frame the
// tagger sees.
func ProjectConceptFrame(ctx context.Context, store issues.MemoryStore, logger *slog.Logger) ai.ConceptFrame {
	if store == nil {
		return ai.ConceptFrame{}
	}

	frame := ai.ConceptFrame{}
	overview, err := store.GetActiveOverview(ctx)
	switch {
	case err == nil:
		frame.Overview = strings.TrimSpace(overview.Body)
	case errors.Is(err, issues.ErrMemoryNotFound):
		// No overview set yet — the concepts alone still ground tagging.
	default:
		if logger != nil {
			logger.WarnContext(ctx, "load project overview for tagging frame failed", "error", err)
		}
	}

	concepts, err := store.ListMemories(ctx, issues.MemoryListOptions{
		Status: domain.MemoryStatusActive,
		Kind:   domain.MemoryKindConcept,
	})
	if err != nil {
		if logger != nil {
			logger.WarnContext(ctx, "load project concepts for tagging frame failed", "error", err)
		}
		return frame
	}

	// Most-reinforced concepts lead — they are the nouns the corpus orbits most —
	// then newest, then subject tag for a stable order. Cap before profiling.
	sort.SliceStable(concepts, func(i, j int) bool {
		if concepts[i].ReinforcementCount != concepts[j].ReinforcementCount {
			return concepts[i].ReinforcementCount > concepts[j].ReinforcementCount
		}
		if !concepts[i].CreatedAt.Equal(concepts[j].CreatedAt) {
			return concepts[i].CreatedAt.After(concepts[j].CreatedAt)
		}
		return concepts[i].SubjectTag < concepts[j].SubjectTag
	})
	if len(concepts) > conceptFrameMaxConcepts {
		concepts = concepts[:conceptFrameMaxConcepts]
	}

	digests := make([]ai.ConceptDigest, 0, len(concepts))
	for _, concept := range concepts {
		tag := strings.TrimSpace(concept.SubjectTag)
		if tag == "" {
			continue
		}
		digests = append(digests, ai.ConceptDigest{
			SubjectTag: tag,
			Profile:    truncateProfile(conceptProfileText(concept), conceptProfileMaxChars),
		})
	}
	frame.Concepts = digests
	return frame
}

// conceptProfileText is the prose a concept contributes to the frame: its body,
// falling back to its title when the body is empty.
func conceptProfileText(memory domain.Memory) string {
	if body := strings.TrimSpace(memory.Body); body != "" {
		return body
	}
	return strings.TrimSpace(memory.Title)
}

// truncateProfile trims s to at most maxRunes runes (rune-safe, so multibyte
// text is never split mid-character), appending an ellipsis when it cuts.
func truncateProfile(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}

func priorDecisionTitle(memory domain.Memory) string {
	if title := strings.TrimSpace(memory.Title); title != "" {
		return title
	}
	if len(memory.AnchorTags) > 0 {
		return memory.AnchorTags[0]
	}
	return "memory"
}

func (s *IssueEnricher) tagSpecificityMap(ctx context.Context) map[string]*float64 {
	if s.catalog == nil {
		return nil
	}
	tags, err := s.catalog.StoredTags(ctx)
	if err != nil {
		return nil
	}
	values := make(map[string]*float64, len(tags))
	for i := range tags {
		values[tags[i].Name] = tags[i].Specificity
	}
	return values
}

func (s *IssueEnricher) AnalyzeText(ctx context.Context, raw string, opts AnalyzeTextOptions) (AnalyzeTextResult, error) {
	analyzed, candidates, context, err := s.analyzeWithCandidateTaxonomy(ctx, raw, opts.PreferredTags, opts.CandidateMode, !opts.SkipPriorDecisions)
	if err != nil {
		return AnalyzeTextResult{}, err
	}

	tagSpecificity := s.tagSpecificityMap(ctx)
	beforeFloor := len(analyzed.Tags)
	beforeAttenuation := IssueTagScoresFromAnalysis(analyzed.Tags)
	tagScores := attenuateGenericScores(beforeAttenuation, tagSpecificity)

	tagScores = s.decorateAndVerifyTagScores(
		ctx,
		raw,
		Float32VectorToFloat64(analyzed.Embedding.Vector),
		candidates,
		tagScores,
		analyzed.Tags,
		analyzed.Negated,
		tagSpecificity,
		opts.Verify,
	)

	trace := buildAnalysisTrace(raw, candidates, context, analyzed, beforeFloor-len(beforeAttenuation), beforeAttenuation, tagScores, opts.Verify)
	return AnalyzeTextResult{
		Analyzed:     analyzed,
		CandidateSet: candidates,
		TagScores:    tagScores,
		Trace:        trace,
	}, nil
}

func buildAnalysisTrace(raw string, candidates tags.CandidateTaxonomy, context analysisContext, analyzed ai.AnalyzedIssue, floorFiltered int, beforeAttenuation, scores []issues.TagRelevance, verify bool) AnalysisTrace {
	sourceCounts := make(map[string]int)
	hintCount := 0
	for _, candidate := range candidates.Tags {
		if candidate.Hint {
			hintCount++
		}
		for _, source := range candidate.Sources {
			sourceCounts[string(source)]++
		}
	}
	attenuated := make([]string, 0)
	beforeByTag := make(map[string]float64, len(beforeAttenuation))
	for _, score := range beforeAttenuation {
		beforeByTag[score.Tag] = score.Relevance
	}
	verification := AnalysisTraceVerification{Enabled: verify}
	for _, score := range scores {
		if before, ok := beforeByTag[score.Tag]; ok && score.Relevance < before {
			attenuated = append(attenuated, score.Tag)
		}
		switch score.VerificationVerdict {
		case domain.TagVerificationVerdictKeep:
			verification.KeepCount++
		case domain.TagVerificationVerdictDownRank:
			verification.DownRankedCount++
		case domain.TagVerificationVerdictFlagged:
			verification.FlaggedCount++
		}
		if score.Negation != nil && *score.Negation > 0 {
			verification.NegatedCount++
		}
		verification.EvidenceCount += len(score.Evidence)
	}
	return AnalysisTrace{
		Input: AnalysisTraceInput{CharacterCount: len([]rune(raw))},
		CandidateSelection: AnalysisTraceCandidateSelection{
			Mode: candidates.Mode, CandidateCount: len(candidates.Tags), HintCount: hintCount, SourceCounts: sourceCounts,
		},
		Context: AnalysisTraceContext{
			FewShotExampleCount: context.fewShotExampleCount, PriorDecisionCount: context.priorDecisionCount,
			ConceptCount: context.conceptCount, HasProjectOverview: context.hasProjectOverview,
		},
		ModelOutput: AnalysisTraceModelOutput{
			AssignedTagCount: len(analyzed.Tags), NegatedTagCount: len(analyzed.Negated),
			Tags: append([]ai.TagScore(nil), analyzed.Tags...), Negated: append([]ai.NegatedTag(nil), analyzed.Negated...),
		},
		PostProcessing: AnalysisTracePostProcessing{
			RelevanceFloorFilteredCount: floorFiltered, GenericAttenuatedTags: attenuated, PersistedTagCount: len(scores),
		},
		Verification: verification,
	}
}

func IssueTagScoresFromAnalysis(scores []ai.TagScore) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}

	tagScores := make([]issues.TagRelevance, 0, len(scores))
	for _, score := range scores {
		if score.Tag == "" {
			continue
		}
		if score.Relevance < issueTagRelevanceFloor {
			continue
		}
		tagScores = append(tagScores, issues.TagRelevance{
			Tag:         score.Tag,
			Relevance:   score.Relevance,
			Suggested:   score.Suggested,
			Description: strings.TrimSpace(score.Description),
		})
	}
	return tagScores
}
