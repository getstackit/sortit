package ai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

var ErrNotConfigured = errors.New("ai analyzer not configured")

type Analyzer struct {
	tagger   Tagger
	embedder Embedder
}

func NewAnalyzer(tagger Tagger, embedder Embedder) *Analyzer {
	return &Analyzer{
		tagger:   tagger,
		embedder: embedder,
	}
}

func (a *Analyzer) AnalyzeIssueData(ctx context.Context, text string, tags []Tag) (AnalyzedIssue, error) {
	if a == nil || a.tagger == nil || a.embedder == nil {
		return AnalyzedIssue{}, ErrNotConfigured
	}

	scores, err := a.tagger.Score(ctx, text, tags)
	if err != nil {
		return AnalyzedIssue{}, fmt.Errorf("score tags: %w", err)
	}

	embedding, err := a.embedder.EmbedText(ctx, text)
	if err != nil {
		return AnalyzedIssue{}, fmt.Errorf("embed text: %w", err)
	}

	return AnalyzedIssue{
		Tags:      normalizeScores(scores, tags),
		Embedding: embedding,
		Tagger: ModelInfo{
			Provider: a.tagger.Provider(),
			Model:    a.tagger.Model(),
		},
		Embedder: ModelInfo{
			Provider: a.embedder.Provider(),
			Model:    a.embedder.Model(),
		},
	}, nil
}

func (a *Analyzer) AnalyzeIssue(ctx context.Context, text string, tags []Tag) (IssueAnalysis, error) {
	analyzed, err := a.AnalyzeIssueData(ctx, text, tags)
	if err != nil {
		return IssueAnalysis{}, err
	}

	return IssueAnalysis{
		Tags:      analyzed.Tags,
		Embedding: analyzed.Embedding.Info,
		Tagger:    analyzed.Tagger,
		Embedder:  analyzed.Embedder,
	}, nil
}

func (a *Analyzer) EmbedText(ctx context.Context, text string) (EmbeddingResult, error) {
	if a == nil || a.embedder == nil {
		return EmbeddingResult{}, ErrNotConfigured
	}
	return a.embedder.EmbedText(ctx, text)
}

func normalizeScores(scores []TagScore, taxonomy []Tag) []TagScore {
	taxonomyNames := make(map[string]string, len(taxonomy))
	for _, tag := range taxonomy {
		name := normalizeTagName(tag.Name)
		if name == "" {
			continue
		}
		taxonomyNames[name] = tag.Name
	}

	merged := make(map[string]TagScore, len(scores))
	for _, score := range scores {
		tagName := normalizeTagName(score.Tag)
		if tagName == "" {
			continue
		}

		relevance := minFloat(1, maxFloat(0, score.Relevance))
		normalized := TagScore{
			Tag:         tagName,
			Relevance:   math.Round(relevance*1000) / 1000,
			Suggested:   score.Suggested,
			Description: strings.TrimSpace(score.Description),
		}

		if exactName, ok := taxonomyNames[tagName]; ok {
			normalized.Tag = exactName
			normalized.Suggested = false
			normalized.Description = ""
		}

		existing, ok := merged[strings.ToLower(normalized.Tag)]
		if !ok || normalized.Relevance > existing.Relevance {
			merged[strings.ToLower(normalized.Tag)] = normalized
			continue
		}
		if existing.Description == "" && normalized.Description != "" {
			existing.Description = normalized.Description
		}
		existing.Suggested = existing.Suggested || normalized.Suggested
		merged[strings.ToLower(normalized.Tag)] = existing
	}

	normalized := make([]TagScore, 0, len(merged))
	for _, score := range merged {
		normalized = append(normalized, score)
	}

	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Relevance == normalized[j].Relevance {
			return normalized[i].Tag < normalized[j].Tag
		}
		return normalized[i].Relevance > normalized[j].Relevance
	})
	return normalized
}

func normalizeTagName(tag string) string {
	tag = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(tag)), " "))
	return tag
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
