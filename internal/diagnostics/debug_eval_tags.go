package diagnostics

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"sortit/internal/ai"
	"sortit/internal/domain"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

//go:embed testdata/tag_eval_fixture.json
var syntheticDebugTagEvalFixture []byte

//go:embed testdata/tag_eval_fixture_corpus.json
var corpusDebugTagEvalFixture []byte

const debugTagEvalRelevanceFloor = 0.08

const (
	debugTagEvalDefaultFixture   = "corpus"
	debugTagEvalSyntheticFixture = "synthetic"
)

type DebugTagEvalCase struct {
	SourceIssueID string   `json:"sourceIssueId,omitempty"`
	Category      string   `json:"category,omitempty"`
	ID            string   `json:"id"`
	Text          string   `json:"text"`
	ExpectedTags  []string `json:"expectedTags"`
}

type DebugEvalTagsCaseResult struct {
	SourceIssueID       string     `json:"sourceIssueId,omitempty"`
	Category            string     `json:"category,omitempty"`
	ID                  string     `json:"id"`
	Text                string     `json:"text"`
	ExpectedTags        []string   `json:"expectedTags"`
	ActualTags          []string   `json:"actualTags"`
	AlternateActualTags [][]string `json:"alternateActualTags,omitempty"`
	MissingTags         []string   `json:"missingTags"`
	UnexpectedTags      []string   `json:"unexpectedTags"`
	Precision           float64    `json:"precision"`
	Recall              float64    `json:"recall"`
	ExactMatch          bool       `json:"exactMatch"`
	DownrankedCount     int        `json:"downrankedCount,omitempty"`
	FlaggedCount        int        `json:"flaggedCount,omitempty"`
	NegatedCount        int        `json:"negatedCount,omitempty"`
	StabilityExact      bool       `json:"stabilityExact,omitempty"`
	StabilityJaccard    float64    `json:"stabilityJaccard,omitempty"`
}

type DebugEvalTagsResult struct {
	CandidateMode    tags.CandidateMode        `json:"candidateMode"`
	VerifierEnabled  bool                      `json:"verifierEnabled"`
	Fixture          string                    `json:"fixture"`
	Runs             int                       `json:"runs"`
	CaseCount        int                       `json:"caseCount"`
	Precision        float64                   `json:"precision"`
	Recall           float64                   `json:"recall"`
	ExactMatchCount  int                       `json:"exactMatchCount"`
	DownrankedCount  int                       `json:"downrankedCount,omitempty"`
	FlaggedCount     int                       `json:"flaggedCount,omitempty"`
	NegatedCount     int                       `json:"negatedCount,omitempty"`
	ExactStableCount int                       `json:"exactStableCount,omitempty"`
	ExactStability   float64                   `json:"exactStability,omitempty"`
	AverageStability float64                   `json:"averageStability,omitempty"`
	Cases            []DebugEvalTagsCaseResult `json:"cases"`
}

type DebugEvalTagsHandler struct {
	Analyzer *ai.Analyzer
	Catalog  *tags.CatalogService
	Enricher *issueenrichment.IssueEnricher
	Fixture  []DebugTagEvalCase
}

func (h DebugEvalTagsHandler) Handle(ctx context.Context) (DebugEvalTagsResult, error) {
	return h.HandleFixture(ctx, "", 1, tags.CandidateModeRetrievalShortlist, true)
}

func (h DebugEvalTagsHandler) HandleFixture(ctx context.Context, requestedFixture string, runs int, requestedMode tags.CandidateMode, verify bool) (DebugEvalTagsResult, error) {
	if h.Catalog == nil {
		return DebugEvalTagsResult{}, fmt.Errorf("tag catalog is unavailable")
	}
	runs = max(1, runs)
	mode, err := tags.ParseCandidateMode(string(requestedMode))
	if err != nil {
		return DebugEvalTagsResult{}, err
	}

	fixture, fixtureName, err := h.fixtureCases(requestedFixture)
	if err != nil {
		return DebugEvalTagsResult{}, err
	}

	results := make([]DebugEvalTagsCaseResult, 0, len(fixture))
	totalExpected := 0
	totalActual := 0
	totalTruePositive := 0
	exactMatches := 0
	totalDownranked := 0
	totalFlagged := 0
	totalNegated := 0
	exactStableCount := 0
	totalStability := 0.0
	for _, item := range fixture {
		actualTags, downrankedCount, flaggedCount, negatedCount, err := h.evaluateCase(ctx, item, mode, verify)
		if err != nil {
			return DebugEvalTagsResult{}, err
		}
		missing, unexpected, truePositive := diffEvalTags(item.ExpectedTags, actualTags)
		caseResult := DebugEvalTagsCaseResult{
			SourceIssueID:   item.SourceIssueID,
			Category:        item.Category,
			ID:              item.ID,
			Text:            item.Text,
			ExpectedTags:    append([]string(nil), item.ExpectedTags...),
			ActualTags:      actualTags,
			MissingTags:     missing,
			UnexpectedTags:  unexpected,
			Precision:       roundMetric(ratio(truePositive, len(actualTags))),
			Recall:          roundMetric(ratio(truePositive, len(item.ExpectedTags))),
			ExactMatch:      len(missing) == 0 && len(unexpected) == 0,
			DownrankedCount: downrankedCount,
			FlaggedCount:    flaggedCount,
			NegatedCount:    negatedCount,
		}
		caseResult.StabilityExact = true
		caseResult.StabilityJaccard = 1
		if runs > 1 {
			alternateTags, exactStable, stabilityJaccard, err := h.evaluateCaseStability(ctx, item, mode, actualTags, runs, verify)
			if err != nil {
				return DebugEvalTagsResult{}, err
			}
			caseResult.AlternateActualTags = alternateTags
			caseResult.StabilityExact = exactStable
			caseResult.StabilityJaccard = roundMetric(stabilityJaccard)
		}
		if caseResult.ExactMatch {
			exactMatches++
		}
		if caseResult.StabilityExact {
			exactStableCount++
		}

		results = append(results, caseResult)
		totalExpected += len(item.ExpectedTags)
		totalActual += len(actualTags)
		totalTruePositive += truePositive
		totalDownranked += downrankedCount
		totalFlagged += flaggedCount
		totalNegated += negatedCount
		totalStability += caseResult.StabilityJaccard
	}

	return DebugEvalTagsResult{
		CandidateMode:    mode,
		VerifierEnabled:  verify,
		Fixture:          fixtureName,
		Runs:             runs,
		CaseCount:        len(results),
		Precision:        roundMetric(ratio(totalTruePositive, totalActual)),
		Recall:           roundMetric(ratio(totalTruePositive, totalExpected)),
		ExactMatchCount:  exactMatches,
		DownrankedCount:  totalDownranked,
		FlaggedCount:     totalFlagged,
		NegatedCount:     totalNegated,
		ExactStableCount: exactStableCount,
		ExactStability:   roundMetric(ratio(exactStableCount, len(results))),
		AverageStability: roundMetric(totalStability / float64(max(1, len(results)))),
		Cases:            results,
	}, nil
}

func (h DebugEvalTagsHandler) evaluateCase(ctx context.Context, item DebugTagEvalCase, mode tags.CandidateMode, verify bool) ([]string, int, int, int, error) {
	if h.Enricher == nil {
		actualTags, err := h.evaluateCaseLegacy(ctx, item, mode)
		if err != nil {
			return nil, 0, 0, 0, err
		}
		return actualTags, 0, 0, 0, nil
	}

	analysis, err := h.Enricher.AnalyzeText(ctx, item.Text, issueenrichment.AnalyzeTextOptions{
		CandidateMode: mode,
		Verify:        verify,
	})
	if err != nil {
		return nil, 0, 0, 0, err
	}
	downrankedCount, flaggedCount, negatedCount := countVerificationVerdicts(analysis.TagScores)
	return predictedEvalTagRelevance(analysis.TagScores), downrankedCount, flaggedCount, negatedCount, nil
}

func countVerificationVerdicts(scores []issues.TagRelevance) (int, int, int) {
	downranked := 0
	flagged := 0
	negated := 0
	for _, score := range scores {
		switch score.VerificationVerdict {
		case domain.TagVerificationVerdictDownRank:
			downranked++
		case domain.TagVerificationVerdictFlagged:
			flagged++
		}
		if score.Negation != nil && *score.Negation > 0 {
			negated++
		}
	}
	return downranked, flagged, negated
}

func (h DebugEvalTagsHandler) evaluateCaseStability(ctx context.Context, item DebugTagEvalCase, mode tags.CandidateMode, baseline []string, runs int, verify bool) ([][]string, bool, float64, error) {
	if runs <= 1 {
		return nil, true, 1, nil
	}

	alternateSets := make([][]string, 0)
	seenAlternates := make(map[string]struct{})
	exactStable := true
	totalJaccard := 0.0
	for range runs - 1 {
		repeatedTags, _, _, _, err := h.evaluateCase(ctx, item, mode, verify)
		if err != nil {
			return nil, false, 0, err
		}
		if !equalTagSets(baseline, repeatedTags) {
			exactStable = false
			key := canonicalTagSetKey(repeatedTags)
			if _, ok := seenAlternates[key]; !ok {
				seenAlternates[key] = struct{}{}
				alternateSets = append(alternateSets, append([]string(nil), repeatedTags...))
			}
		}
		totalJaccard += jaccardTagSets(baseline, repeatedTags)
	}
	return alternateSets, exactStable, totalJaccard / float64(runs-1), nil
}

func (h DebugEvalTagsHandler) evaluateCaseLegacy(ctx context.Context, item DebugTagEvalCase, mode tags.CandidateMode) ([]string, error) {
	if h.Analyzer == nil {
		return nil, ai.ErrNotConfigured
	}

	seedEmbedding, err := h.Analyzer.EmbedText(ctx, item.Text)
	if err != nil {
		return nil, err
	}
	candidates, err := h.Catalog.IssueTaxonomyCandidates(ctx, issueenrichment.Float32VectorToFloat64(seedEmbedding.Vector), nil, mode, 15)
	if err != nil {
		return nil, err
	}
	candidates = h.Catalog.AnnotateCandidateHints(ctx, candidates, issueenrichment.Float32VectorToFloat64(seedEmbedding.Vector), 5)

	analyzed, err := h.Analyzer.AnalyzeIssueData(ctx, item.Text, candidates.AITags(), nil, ai.ConceptFrame{})
	if err != nil {
		return nil, err
	}
	return predictedEvalTags(analyzed.Tags), nil
}

func (h DebugEvalTagsHandler) fixtureCases(requestedFixture string) ([]DebugTagEvalCase, string, error) {
	requestedFixture = strings.TrimSpace(strings.ToLower(requestedFixture))
	if len(h.Fixture) > 0 {
		if requestedFixture != "" && requestedFixture != "inline" {
			return nil, "", fmt.Errorf("fixture %q is unavailable when inline fixture cases are configured", requestedFixture)
		}
		fixture, err := normalizeDebugTagEvalCases(h.Fixture)
		return fixture, "inline", err
	}

	if requestedFixture == "" || requestedFixture == "default" {
		requestedFixture = debugTagEvalDefaultFixture
	}

	fixtureBytes, err := debugTagEvalFixtureBytes(requestedFixture)
	if err != nil {
		return nil, "", err
	}

	var fixture []DebugTagEvalCase
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		return nil, "", fmt.Errorf("decode %s tag eval fixture: %w", requestedFixture, err)
	}

	normalized, err := normalizeDebugTagEvalCases(fixture)
	if err != nil {
		return nil, "", err
	}
	return normalized, requestedFixture, nil
}

func debugTagEvalFixtureBytes(name string) ([]byte, error) {
	switch name {
	case debugTagEvalDefaultFixture, "real", "real-corpus":
		return corpusDebugTagEvalFixture, nil
	case debugTagEvalSyntheticFixture:
		return syntheticDebugTagEvalFixture, nil
	default:
		return nil, fmt.Errorf("unknown tag eval fixture %q", name)
	}
}

func normalizeDebugTagEvalCases(items []DebugTagEvalCase) ([]DebugTagEvalCase, error) {
	out := make([]DebugTagEvalCase, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		sourceIssueID := strings.TrimSpace(item.SourceIssueID)
		category := strings.TrimSpace(item.Category)
		id := strings.TrimSpace(item.ID)
		text := strings.TrimSpace(item.Text)
		if id == "" {
			return nil, fmt.Errorf("fixture case id is required")
		}
		if text == "" {
			return nil, fmt.Errorf("fixture case %q text is required", id)
		}
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("fixture case %q is duplicated", id)
		}
		seen[id] = struct{}{}

		out = append(out, DebugTagEvalCase{
			SourceIssueID: sourceIssueID,
			Category:      category,
			ID:            id,
			Text:          text,
			ExpectedTags:  normalizeTagNames(item.ExpectedTags),
		})
	}
	return out, nil
}

func predictedEvalTags(scores []ai.TagScore) []string {
	names := make([]string, 0, len(scores))
	for _, score := range scores {
		names = appendNormalizedEvalTag(names, score.Tag, score.Relevance)
	}
	return names
}

func predictedEvalTagRelevance(scores []issues.TagRelevance) []string {
	names := make([]string, 0, len(scores))
	for _, score := range scores {
		names = appendNormalizedEvalTag(names, score.Tag, score.Relevance)
	}
	return names
}

func appendNormalizedEvalTag(existing []string, raw string, relevance float64) []string {
	if relevance < debugTagEvalRelevanceFloor {
		return existing
	}
	name := domain.NormalizeTagName(raw)
	if name == "" {
		return existing
	}
	for _, existingName := range existing {
		if existingName == name {
			return existing
		}
	}
	return append(existing, name)
}

func diffEvalTags(expected []string, actual []string) ([]string, []string, int) {
	expectedSet := make(map[string]struct{}, len(expected))
	for _, name := range expected {
		expectedSet[name] = struct{}{}
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, name := range actual {
		actualSet[name] = struct{}{}
	}

	missing := make([]string, 0)
	unexpected := make([]string, 0)
	truePositive := 0
	for _, name := range expected {
		if _, ok := actualSet[name]; ok {
			truePositive++
			continue
		}
		missing = append(missing, name)
	}
	for _, name := range actual {
		if _, ok := expectedSet[name]; ok {
			continue
		}
		unexpected = append(unexpected, name)
	}
	slices.Sort(missing)
	slices.Sort(unexpected)
	return missing, unexpected, truePositive
}

func equalTagSets(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftSet := make(map[string]struct{}, len(left))
	for _, name := range left {
		leftSet[name] = struct{}{}
	}
	for _, name := range right {
		if _, ok := leftSet[name]; !ok {
			return false
		}
	}
	return true
}

func jaccardTagSets(left []string, right []string) float64 {
	if len(left) == 0 && len(right) == 0 {
		return 1
	}
	leftSet := make(map[string]struct{}, len(left))
	for _, name := range left {
		leftSet[name] = struct{}{}
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, name := range right {
		rightSet[name] = struct{}{}
	}
	intersection := 0
	for name := range leftSet {
		if _, ok := rightSet[name]; ok {
			intersection++
		}
	}
	union := len(leftSet)
	for name := range rightSet {
		if _, ok := leftSet[name]; !ok {
			union++
		}
	}
	return float64(intersection) / float64(union)
}

func canonicalTagSetKey(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	sorted := append([]string(nil), tags...)
	slices.Sort(sorted)
	return strings.Join(sorted, "\x00")
}

func normalizeTagNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, raw := range names {
		name := domain.NormalizeTagName(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func roundMetric(value float64) float64 {
	return math.Round(value*1000) / 1000
}
