package services

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"splat/internal/ai"
	"splat/internal/domain"
	"splat/internal/issues"
	"splat/internal/vectors"
)

type IssueEnricher struct {
	analyzer   *ai.Analyzer
	catalog    *CatalogService
	logger     *slog.Logger
	exemplars  *ExemplarPool
}

type AnalyzeTextOptions struct {
	PreferredTags []string
	CandidateMode CandidateMode
	Verify        bool
}

type AnalyzeTextResult struct {
	Analyzed     ai.AnalyzedIssue
	CandidateSet CandidateTaxonomy
	TagScores    []issues.TagRelevance
}

const issueEnrichmentTimeout = 20 * time.Second
const persistedIssueHintLimit = 5

func NewIssueEnricher(analyzer *ai.Analyzer, catalog *CatalogService, logger *slog.Logger) *IssueEnricher {
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

func (s *IssueEnricher) AnalyzeCreateInput(ctx context.Context, input issues.CreateInput) (issues.CreateInput, error) {
	ctx, cancel := context.WithTimeout(ctx, issueEnrichmentTimeout)
	defer cancel()

	start := time.Now()
	s.logger.InfoContext(ctx, "analyzing create input", "tag_count", len(input.Tags))

	analysis, err := s.AnalyzeText(ctx, input.Raw, AnalyzeTextOptions{
		PreferredTags: input.Tags,
		CandidateMode: CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		return issues.CreateInput{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, CatalogTagsFromAnalysis(analysis.CandidateSet.AITags(), input.Tags, analysis.Analyzed.Tags)); err != nil {
		return issues.CreateInput{}, err
	}

	tagSpecificity := s.tagSpecificityMap(ctx)
	input.TagScores = analysis.TagScores
	input.Embedding = Float32VectorToFloat64(analysis.Analyzed.Embedding.Vector)
	if len(input.Tags) == 0 {
		input.Tags = issues.DisplayTagsWithSpecificity(nil, input.TagScores, tagSpecificity)
	}

	s.logger.InfoContext(ctx, "create input analyzed",
		"scored_tags", len(input.TagScores),
		"has_embedding", len(input.Embedding) > 0,
		"duration", time.Since(start).Round(time.Millisecond),
	)
	return input, nil
}

func (s *IssueEnricher) AnalyzeRefineInput(ctx context.Context, issue issues.Issue, postRaw string, createdBy string) (issues.RefineInput, error) {
	ctx, cancel := context.WithTimeout(ctx, issueEnrichmentTimeout)
	defer cancel()

	postRaw = strings.TrimSpace(postRaw)
	if postRaw == "" {
		return issues.RefineInput{}, fmt.Errorf("post raw is required")
	}

	discussionTexts := make([]string, 0, len(issue.Discussion)+1)
	for _, post := range issue.Discussion {
		text := strings.TrimSpace(post.Raw)
		if text == "" {
			continue
		}
		discussionTexts = append(discussionTexts, text)
	}
	discussionTexts = append(discussionTexts, postRaw)

	canonicalRaw, err := s.analyzer.CanonicalizeDiscussion(ctx, discussionTexts)
	if err != nil {
		return issues.RefineInput{}, err
	}
	canonicalRaw = strings.TrimSpace(canonicalRaw)
	if canonicalRaw == "" {
		return issues.RefineInput{}, fmt.Errorf("canonical raw is required")
	}

	analysis, err := s.AnalyzeText(ctx, canonicalRaw, AnalyzeTextOptions{
		CandidateMode: CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		return issues.RefineInput{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, CatalogTagsFromAnalysis(analysis.CandidateSet.AITags(), nil, analysis.Analyzed.Tags)); err != nil {
		return issues.RefineInput{}, err
	}

	tagSpecificity := s.tagSpecificityMap(ctx)
	tagScores := analysis.TagScores

	return issues.RefineInput{
		PostRaw:      postRaw,
		CanonicalRaw: canonicalRaw,
		Tags:         issues.DisplayTagsWithSpecificity(nil, tagScores, tagSpecificity),
		CreatedBy:    createdBy,
		TagScores:    tagScores,
		Embedding:    Float32VectorToFloat64(analysis.Analyzed.Embedding.Vector),
	}, nil
}

func (s *IssueEnricher) AnalyzePersistedIssue(
	ctx context.Context,
	issue issues.Issue,
	targetSequence int,
) (issues.IssueFieldUpdate, error) {
	ctx, cancel := context.WithTimeout(ctx, issueEnrichmentTimeout)
	defer cancel()

	start := time.Now()
	s.logger.InfoContext(ctx, "enriching persisted issue",
		"issue_id", issue.ID,
		"target_sequence", targetSequence,
	)

	targetSequence = max(1, targetSequence)
	discussionTexts := make([]string, 0, len(issue.Discussion))
	for _, post := range issue.Discussion {
		if post.Sequence > targetSequence {
			continue
		}
		text := strings.TrimSpace(post.Raw)
		if text == "" {
			continue
		}
		discussionTexts = append(discussionTexts, text)
	}
	if len(discussionTexts) == 0 {
		text := strings.TrimSpace(issue.Raw)
		if text == "" {
			return issues.IssueFieldUpdate{}, fmt.Errorf("issue raw is required")
		}
		discussionTexts = append(discussionTexts, text)
	}

	canonicalRaw := discussionTexts[0]
	var err error
	if targetSequence > 1 {
		canonicalRaw, err = s.analyzer.CanonicalizeDiscussion(ctx, discussionTexts)
		if err != nil {
			return issues.IssueFieldUpdate{}, err
		}
		canonicalRaw = strings.TrimSpace(canonicalRaw)
		if canonicalRaw == "" {
			return issues.IssueFieldUpdate{}, fmt.Errorf("canonical raw is required")
		}
	}

	analysis, err := s.AnalyzeText(ctx, canonicalRaw, AnalyzeTextOptions{
		CandidateMode: CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		return issues.IssueFieldUpdate{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, CatalogTagsFromAnalysis(analysis.CandidateSet.AITags(), nil, analysis.Analyzed.Tags)); err != nil {
		return issues.IssueFieldUpdate{}, err
	}

	s.logger.InfoContext(ctx, "persisted issue enriched",
		"issue_id", issue.ID,
		"scored_tags", len(analysis.Analyzed.Tags),
		"has_embedding", len(analysis.Analyzed.Embedding.Vector) > 0,
		"duration", time.Since(start).Round(time.Millisecond),
	)

	status := issues.EnrichmentStatusComplete
	emptyError := ""
	tagSpecificity := s.tagSpecificityMap(ctx)
	tagScores := analysis.TagScores

	return issues.IssueFieldUpdate{
		Raw:                      &canonicalRaw,
		Tags:                     issues.DisplayTagsWithSpecificity(nil, tagScores, tagSpecificity),
		TagScores:                tagScores,
		Embedding:                Float32VectorToFloat64(analysis.Analyzed.Embedding.Vector),
		EnrichmentStatus:         &status,
		EnrichmentError:          &emptyError,
		EnrichmentTargetSequence: &targetSequence,
	}, nil
}

func (s *IssueEnricher) AnalyzeCombineInput(
	ctx context.Context,
	sourceIssues []issues.Issue,
	createdBy string,
	note string,
) (issues.CombineInput, error) {
	ctx, cancel := context.WithTimeout(ctx, issueEnrichmentTimeout)
	defer cancel()

	if len(sourceIssues) < 2 {
		return issues.CombineInput{}, fmt.Errorf("at least two source issues are required")
	}

	parts := make([]string, 0, len(sourceIssues)+1)
	sourceIDs := make([]string, 0, len(sourceIssues))
	for _, issue := range sourceIssues {
		raw := strings.TrimSpace(issue.Raw)
		if raw == "" {
			continue
		}
		parts = append(parts, raw)
		sourceIDs = append(sourceIDs, issue.ID)
	}
	if extra := strings.TrimSpace(note); extra != "" {
		parts = append(parts, "Combination rationale: "+extra)
	}

	canonicalRaw, err := s.analyzer.CanonicalizeDiscussion(ctx, parts)
	if err != nil {
		return issues.CombineInput{}, err
	}
	canonicalRaw = strings.TrimSpace(canonicalRaw)
	if canonicalRaw == "" {
		return issues.CombineInput{}, fmt.Errorf("canonical raw is required")
	}

	analysis, err := s.AnalyzeText(ctx, canonicalRaw, AnalyzeTextOptions{
		CandidateMode: CandidateModeRetrievalShortlist,
		Verify:        true,
	})
	if err != nil {
		return issues.CombineInput{}, err
	}

	if err := s.catalog.EnsureStoredTags(ctx, CatalogTagsFromAnalysis(analysis.CandidateSet.AITags(), nil, analysis.Analyzed.Tags)); err != nil {
		return issues.CombineInput{}, err
	}

	tagSpecificity := s.tagSpecificityMap(ctx)
	tagScores := analysis.TagScores

	return issues.CombineInput{
		SourceIDs: sourceIDs,
		Raw:       canonicalRaw,
		Tags:      issues.DisplayTagsWithSpecificity(nil, tagScores, tagSpecificity),
		CreatedBy: createdBy,
		Note:      note,
		TagScores: tagScores,
		Embedding: Float32VectorToFloat64(analysis.Analyzed.Embedding.Vector),
	}, nil
}

func (s *IssueEnricher) analyzeWithCandidateTaxonomy(ctx context.Context, raw string, preferred []string, mode CandidateMode) (ai.AnalyzedIssue, CandidateTaxonomy, error) {
	freshEmbedding, err := s.analyzer.EmbedText(ctx, raw)
	if err != nil {
		return ai.AnalyzedIssue{}, CandidateTaxonomy{}, fmt.Errorf("embed raw for shortlist: %w", err)
	}
	freshEmbeddingVector := Float32VectorToFloat64(freshEmbedding.Vector)

	candidates, err := s.catalog.IssueTaxonomyCandidates(ctx, freshEmbeddingVector, preferred, mode, 15)
	if err != nil {
		return ai.AnalyzedIssue{}, CandidateTaxonomy{}, err
	}
	candidates = s.catalog.AnnotateCandidateHints(ctx, candidates, freshEmbeddingVector, persistedIssueHintLimit)

	// Select few-shot examples using the fresh embedding and candidate tag names.
	var examples []ai.FewShotExample
	if s.exemplars != nil && s.analyzer != nil {
		candidateTagNames := make([]string, 0, len(candidates.Tags))
		for _, tag := range candidates.Tags {
			candidateTagNames = append(candidateTagNames, tag.Name)
		}
		examples = s.exemplars.Select(ctx, s.analyzer, freshEmbeddingVector, candidateTagNames, 3)
	}

	analyzed, err := s.analyzer.AnalyzeIssueData(ctx, raw, candidates.AITags(), examples)
	if err != nil {
		return ai.AnalyzedIssue{}, CandidateTaxonomy{}, err
	}
	return analyzed, candidates, nil
}

const (
	genericSpecificityThreshold = 0.3
	genericMultiplier           = 0.6
	defaultSpecificity          = 0.5
	issueTagRelevanceFloor      = 0.08
	verifierWeakAlignment       = 0.16
	verifierFlaggedAlignment    = 0.08
	verifierDominatingAlignment = 0.35
	verifierDominanceMargin     = 0.18
	verifierSpecificityMargin   = 0.10
	verifierDownrankMultiplier  = 0.75
)

// attenuateGenericScores reduces relevance of low-specificity tags when more
// specific tags are also present, so that specific tags rank higher.
// tagSpecificity maps tag name → specificity score (nil = unscored, treated as 0.5).
func attenuateGenericScores(scores []issues.TagRelevance, tagSpecificity map[string]*float64) []issues.TagRelevance {
	if len(scores) == 0 {
		return scores
	}

	hasSpecific := false
	for _, s := range scores {
		if tagSpecificityValue(s.Tag, tagSpecificity) >= genericSpecificityThreshold && s.Relevance > 0 {
			hasSpecific = true
			break
		}
	}
	if !hasSpecific {
		return scores
	}

	out := make([]issues.TagRelevance, len(scores))
	for i, s := range scores {
		out[i] = s
		if tagSpecificityValue(s.Tag, tagSpecificity) < genericSpecificityThreshold {
			out[i].Relevance = s.Relevance * genericMultiplier
		}
	}
	return out
}

func tagSpecificityValue(name string, specificity map[string]*float64) float64 {
	if specificity == nil {
		return defaultSpecificity
	}
	p, ok := specificity[name]
	if !ok || p == nil {
		return defaultSpecificity
	}
	return *p
}

func (s *IssueEnricher) tagSpecificityMap(ctx context.Context) map[string]*float64 {
	if s.catalog == nil {
		return nil
	}
	tags, err := s.catalog.StoredTags(ctx)
	if err != nil {
		return nil
	}
	m := make(map[string]*float64, len(tags))
	for i := range tags {
		m[tags[i].Name] = tags[i].Specificity
	}
	return m
}

func (s *IssueEnricher) AnalyzeText(ctx context.Context, raw string, opts AnalyzeTextOptions) (AnalyzeTextResult, error) {
	analyzed, candidates, err := s.analyzeWithCandidateTaxonomy(ctx, raw, opts.PreferredTags, opts.CandidateMode)
	if err != nil {
		return AnalyzeTextResult{}, err
	}

	tagSpecificity := s.tagSpecificityMap(ctx)
	tagScores := attenuateGenericScores(IssueTagScoresFromAnalysis(analyzed.Tags), tagSpecificity)
	tagScores = s.decorateAndVerifyTagScores(
		ctx,
		Float32VectorToFloat64(analyzed.Embedding.Vector),
		candidates,
		tagScores,
		tagSpecificity,
		opts.Verify,
	)

	return AnalyzeTextResult{
		Analyzed:     analyzed,
		CandidateSet: candidates,
		TagScores:    tagScores,
	}, nil
}

func IssueTagScoresFromAnalysis(scores []ai.TagScore) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}

	tagScores := make([]issues.TagRelevance, 0, len(scores))
	for _, score := range scores {
		name := score.Tag
		if name == "" {
			continue
		}
		if score.Relevance < issueTagRelevanceFloor {
			continue
		}
		tagScores = append(tagScores, issues.TagRelevance{
			Tag:         name,
			Relevance:   score.Relevance,
			Suggested:   score.Suggested,
			Description: strings.TrimSpace(score.Description),
		})
	}
	return tagScores
}

type verifierCandidate struct {
	Name        string
	Alignment   float64
	Specificity float64
	Sources     []string
}

func (s *IssueEnricher) decorateAndVerifyTagScores(
	ctx context.Context,
	issueEmbedding []float64,
	candidates CandidateTaxonomy,
	scores []issues.TagRelevance,
	tagSpecificity map[string]*float64,
	verify bool,
) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}

	candidateByName := make(map[string]CandidateTag, len(candidates.Tags))
	for _, candidate := range candidates.Tags {
		name := normalizeCatalogTagName(candidate.Name)
		if name == "" {
			continue
		}
		candidateByName[name] = candidate
	}

	storedTags, err := s.catalog.StoredTags(ctx)
	if err != nil {
		storedTags = nil
	}
	tagEmbeddingByName := make(map[string][]float64, len(storedTags))
	for _, tag := range storedTags {
		name := normalizeCatalogTagName(tag.Name)
		if name == "" || len(tag.Embedding) == 0 {
			continue
		}
		tagEmbeddingByName[name] = append([]float64(nil), tag.Embedding...)
	}

	assigned := make(map[string]struct{}, len(scores))
	for _, score := range scores {
		assigned[normalizeCatalogTagName(score.Tag)] = struct{}{}
	}

	unassigned := make([]verifierCandidate, 0, len(candidates.Tags))
	if len(issueEmbedding) > 0 {
		for _, candidate := range candidates.Tags {
			name := normalizeCatalogTagName(candidate.Name)
			if name == "" {
				continue
			}
			if _, ok := assigned[name]; ok {
				continue
			}
			embedding := tagEmbeddingByName[name]
			if len(embedding) == 0 {
				continue
			}
			alignment := roundVerifierMetric(vectors.CosineSimilarity(issueEmbedding, embedding))
			unassigned = append(unassigned, verifierCandidate{
				Name:        name,
				Alignment:   alignment,
				Specificity: tagSpecificityValue(name, tagSpecificity),
				Sources:     candidateSourceStrings(candidate.Sources),
			})
		}
		slices.SortFunc(unassigned, func(a, b verifierCandidate) int {
			if c := cmp.Compare(b.Alignment, a.Alignment); c != 0 {
				return c
			}
			if c := cmp.Compare(b.Specificity, a.Specificity); c != 0 {
				return c
			}
			return cmp.Compare(a.Name, b.Name)
		})
	}

	out := issuesDisplayCopyTagScores(scores)
	for i := range out {
		name := normalizeCatalogTagName(out[i].Tag)
		if candidate, ok := candidateByName[name]; ok {
			out[i].CandidateSources = candidateSourceStrings(candidate.Sources)
		}
		if specificity, ok := tagSpecificity[name]; ok && specificity != nil {
			out[i].Specificity = cloneMetricPointer(*specificity)
		}
		if len(issueEmbedding) > 0 {
			if embedding := tagEmbeddingByName[name]; len(embedding) > 0 {
				out[i].Alignment = cloneMetricPointer(vectors.CosineSimilarity(issueEmbedding, embedding))
			}
		}
		if !verify {
			continue
		}

		assignedSpecificity := tagSpecificityValue(name, tagSpecificity)
		dominating, gap := bestDominatingCandidate(out[i], assignedSpecificity, unassigned)
		if dominating != nil {
			out[i].DominatedBy = dominating.Name
			out[i].DominanceGap = cloneMetricPointer(gap)
		}

		switch {
		case out[i].Alignment != nil && *out[i].Alignment < verifierFlaggedAlignment && out[i].Relevance >= 0.4:
			out[i].VerificationVerdict = domain.TagVerificationVerdictFlagged
			if dominating != nil {
				out[i].VerificationReason = fmt.Sprintf("very weak alignment; nearby unassigned %s is better aligned", dominating.Name)
			} else {
				out[i].VerificationReason = "high relevance but very weak embedding alignment"
			}
		case dominating != nil && out[i].Alignment != nil && *out[i].Alignment < verifierWeakAlignment:
			out[i].VerificationVerdict = domain.TagVerificationVerdictDownRank
			out[i].VerificationReason = fmt.Sprintf("dominated by nearby unassigned %s", dominating.Name)
			out[i].Relevance = roundRelevance(max(issueTagRelevanceFloor, out[i].Relevance*verifierDownrankMultiplier))
		case anchorOnlyCandidate(out[i].CandidateSources) && out[i].Alignment != nil && *out[i].Alignment < verifierWeakAlignment && out[i].Relevance >= 0.35:
			out[i].VerificationVerdict = domain.TagVerificationVerdictFlagged
			out[i].VerificationReason = "anchor-only candidate with weak embedding alignment"
		default:
			out[i].VerificationVerdict = domain.TagVerificationVerdictKeep
			if dominating != nil {
				out[i].VerificationReason = fmt.Sprintf("kept despite stronger nearby %s", dominating.Name)
			}
		}
	}

	slices.SortStableFunc(out, func(a, b issues.TagRelevance) int {
		if c := cmp.Compare(b.Relevance, a.Relevance); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})
	return out
}

func bestDominatingCandidate(
	score issues.TagRelevance,
	assignedSpecificity float64,
	unassigned []verifierCandidate,
) (*verifierCandidate, float64) {
	if score.Alignment == nil {
		return nil, 0
	}
	bestIndex := -1
	bestGap := 0.0
	for i, candidate := range unassigned {
		gap := candidate.Alignment - *score.Alignment
		if candidate.Alignment < verifierDominatingAlignment || gap < verifierDominanceMargin {
			continue
		}
		if candidate.Specificity+0.001 < assignedSpecificity+verifierSpecificityMargin &&
			assignedSpecificity >= genericSpecificityThreshold &&
			!anchorOnlyCandidate(score.CandidateSources) {
			continue
		}
		if bestIndex == -1 ||
			gap > bestGap ||
			(gap == bestGap && candidate.Specificity > unassigned[bestIndex].Specificity) {
			bestIndex = i
			bestGap = gap
		}
	}
	if bestIndex == -1 {
		return nil, 0
	}
	return &unassigned[bestIndex], roundVerifierMetric(bestGap)
}

func candidateSourceStrings(sources []CandidateSource) []string {
	if len(sources) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(sources))
	out := make([]string, 0, len(sources))
	for _, source := range []CandidateSource{
		CandidateSourceExplicit,
		CandidateSourceRetrieval,
		CandidateSourceAnchor,
		CandidateSourceFullCatalog,
	} {
		for _, candidate := range sources {
			if candidate != source {
				continue
			}
			label := string(candidate)
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			out = append(out, label)
		}
	}
	for _, source := range sources {
		label := string(source)
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func anchorOnlyCandidate(sources []string) bool {
	return len(sources) == 1 && sources[0] == string(CandidateSourceAnchor)
}

func issuesDisplayCopyTagScores(scores []issues.TagRelevance) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}
	out := make([]issues.TagRelevance, len(scores))
	for i, score := range scores {
		out[i] = score
		out[i].CandidateSources = append([]string(nil), score.CandidateSources...)
		out[i].Alignment = copyMetricPointer(score.Alignment)
		out[i].Specificity = copyMetricPointer(score.Specificity)
		out[i].DominanceGap = copyMetricPointer(score.DominanceGap)
	}
	return out
}

func cloneMetricPointer(value float64) *float64 {
	rounded := roundVerifierMetric(value)
	return &rounded
}

func copyMetricPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return cloneMetricPointer(*value)
}

func roundVerifierMetric(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func roundRelevance(value float64) float64 {
	return math.Round(value*100) / 100
}

func Float32VectorToFloat64(values []float32) []float64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]float64, len(values))
	for i, value := range values {
		out[i] = float64(value)
	}
	return out
}
