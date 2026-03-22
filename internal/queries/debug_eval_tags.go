package queries

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"splat/internal/ai"
	"splat/internal/domain"
	"splat/internal/issues"
	"splat/internal/services"
)

//go:embed testdata/tag_eval_fixture.json
var defaultDebugTagEvalFixture []byte

const debugTagEvalRelevanceFloor = 0.08

type DebugTagEvalCase struct {
	ID           string   `json:"id"`
	Text         string   `json:"text"`
	ExpectedTags []string `json:"expectedTags"`
}

type DebugEvalTagsCaseResult struct {
	ID             string   `json:"id"`
	Text           string   `json:"text"`
	ExpectedTags   []string `json:"expectedTags"`
	ActualTags     []string `json:"actualTags"`
	MissingTags    []string `json:"missingTags"`
	UnexpectedTags []string `json:"unexpectedTags"`
	Precision      float64  `json:"precision"`
	Recall         float64  `json:"recall"`
	ExactMatch     bool     `json:"exactMatch"`
}

type DebugEvalTagsResult struct {
	Fixture         string                    `json:"fixture"`
	CaseCount       int                       `json:"caseCount"`
	Precision       float64                   `json:"precision"`
	Recall          float64                   `json:"recall"`
	ExactMatchCount int                       `json:"exactMatchCount"`
	Cases           []DebugEvalTagsCaseResult `json:"cases"`
}

type DebugEvalTagsHandler struct {
	Analyzer *ai.Analyzer
	Catalog  *services.CatalogService
	Enricher *services.IssueEnricher
	Fixture  []DebugTagEvalCase
}

func (h DebugEvalTagsHandler) Handle(ctx context.Context) (DebugEvalTagsResult, error) {
	if h.Catalog == nil {
		return DebugEvalTagsResult{}, fmt.Errorf("tag catalog is unavailable")
	}

	fixture, fixtureName, err := h.fixtureCases()
	if err != nil {
		return DebugEvalTagsResult{}, err
	}

	taxonomy, err := h.Catalog.IssueTaxonomy(ctx, nil)
	if err != nil {
		return DebugEvalTagsResult{}, err
	}

	results := make([]DebugEvalTagsCaseResult, 0, len(fixture))
	totalExpected := 0
	totalActual := 0
	totalTruePositive := 0
	exactMatches := 0
	for _, item := range fixture {
		actualTags, err := h.evaluateCase(ctx, item, taxonomy)
		if err != nil {
			return DebugEvalTagsResult{}, err
		}
		missing, unexpected, truePositive := diffEvalTags(item.ExpectedTags, actualTags)
		caseResult := DebugEvalTagsCaseResult{
			ID:             item.ID,
			Text:           item.Text,
			ExpectedTags:   append([]string(nil), item.ExpectedTags...),
			ActualTags:     actualTags,
			MissingTags:    missing,
			UnexpectedTags: unexpected,
			Precision:      roundMetric(ratio(truePositive, len(actualTags))),
			Recall:         roundMetric(ratio(truePositive, len(item.ExpectedTags))),
			ExactMatch:     len(missing) == 0 && len(unexpected) == 0,
		}
		if caseResult.ExactMatch {
			exactMatches++
		}

		results = append(results, caseResult)
		totalExpected += len(item.ExpectedTags)
		totalActual += len(actualTags)
		totalTruePositive += truePositive
	}

	return DebugEvalTagsResult{
		Fixture:         fixtureName,
		CaseCount:       len(results),
		Precision:       roundMetric(ratio(totalTruePositive, totalActual)),
		Recall:          roundMetric(ratio(totalTruePositive, totalExpected)),
		ExactMatchCount: exactMatches,
		Cases:           results,
	}, nil
}

func (h DebugEvalTagsHandler) evaluateCase(ctx context.Context, item DebugTagEvalCase, taxonomy []ai.Tag) ([]string, error) {
	if h.Enricher != nil {
		fields, err := h.Enricher.AnalyzePersistedIssue(ctx, issues.Issue{
			ID:  item.ID,
			Raw: item.Text,
		}, 1)
		if err != nil {
			return nil, err
		}
		return predictedEvalTagRelevance(fields.TagScores), nil
	}
	if h.Analyzer == nil {
		return nil, ai.ErrNotConfigured
	}

	analyzed, err := h.Analyzer.AnalyzeIssueData(ctx, item.Text, taxonomy)
	if err != nil {
		return nil, err
	}
	return predictedEvalTags(analyzed.Tags), nil
}

func (h DebugEvalTagsHandler) fixtureCases() ([]DebugTagEvalCase, string, error) {
	if len(h.Fixture) > 0 {
		fixture, err := normalizeDebugTagEvalCases(h.Fixture)
		return fixture, "inline", err
	}

	var fixture []DebugTagEvalCase
	if err := json.Unmarshal(defaultDebugTagEvalFixture, &fixture); err != nil {
		return nil, "", fmt.Errorf("decode default tag eval fixture: %w", err)
	}

	normalized, err := normalizeDebugTagEvalCases(fixture)
	if err != nil {
		return nil, "", err
	}
	return normalized, "default", nil
}

func normalizeDebugTagEvalCases(items []DebugTagEvalCase) ([]DebugTagEvalCase, error) {
	out := make([]DebugTagEvalCase, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
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
			ID:           id,
			Text:         text,
			ExpectedTags: normalizeTagNames(item.ExpectedTags),
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
