package ai

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"splat/internal/domain"
)

var ErrNotConfigured = errors.New("ai analyzer not configured")

type Analyzer struct {
	tagger        Tagger
	embedder      Embedder
	canonicalizer Canonicalizer
}

func NewAnalyzer(tagger Tagger, embedder Embedder) *Analyzer {
	return NewAnalyzerWithCanonicalizer(tagger, embedder, NewStubCanonicalizer())
}

func NewAnalyzerWithCanonicalizer(tagger Tagger, embedder Embedder, canonicalizer Canonicalizer) *Analyzer {
	return &Analyzer{
		tagger:        tagger,
		embedder:      embedder,
		canonicalizer: canonicalizer,
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

func (a *Analyzer) ScoreTagSpecificity(ctx context.Context, tag Tag, catalog []Tag) (float64, error) {
	if a == nil || a.tagger == nil {
		return 0, ErrNotConfigured
	}
	scorer, ok := a.tagger.(SpecificityScorer)
	if !ok {
		return 0, fmt.Errorf("tagger does not support specificity scoring")
	}
	return scorer.ScoreSpecificity(ctx, tag, catalog)
}

func (a *Analyzer) CanonicalizeDiscussion(ctx context.Context, posts []string) (string, error) {
	if a == nil || a.canonicalizer == nil {
		return "", ErrNotConfigured
	}
	return a.canonicalizer.CanonicalizeDiscussion(ctx, posts)
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
			Relevance:   math.Round(relevance*100) / 100,
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

	slices.SortStableFunc(normalized, func(a, b TagScore) int {
		if c := cmp.Compare(b.Relevance, a.Relevance); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})
	return normalized
}

func normalizeTagName(tag string) string {
	return domain.NormalizeTagName(tag)
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
