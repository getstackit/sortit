package ai

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"sortit/internal/domain"
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

func (a *Analyzer) AnalyzeIssueData(ctx context.Context, text string, tags []Tag, examples []FewShotExample) (AnalyzedIssue, error) {
	if a == nil || a.tagger == nil || a.embedder == nil {
		return AnalyzedIssue{}, ErrNotConfigured
	}

	result, err := a.tagger.Score(ctx, text, tags, examples)
	if err != nil {
		return AnalyzedIssue{}, fmt.Errorf("score tags: %w", err)
	}

	embedding, err := a.embedder.EmbedText(ctx, text)
	if err != nil {
		return AnalyzedIssue{}, fmt.Errorf("embed text: %w", err)
	}

	return AnalyzedIssue{
		Tags:      normalizeScores(result.Tags, tags),
		Negated:   normalizeNegated(result.Negated, tags),
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

func (a *Analyzer) AnalyzeIssue(ctx context.Context, text string, tags []Tag, examples []FewShotExample) (IssueAnalysis, error) {
	analyzed, err := a.AnalyzeIssueData(ctx, text, tags, examples)
	if err != nil {
		return IssueAnalysis{}, err
	}

	return IssueAnalysis{
		Tags:      analyzed.Tags,
		Negated:   analyzed.Negated,
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
			Evidence:    append([]string(nil), score.Evidence...),
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
		if len(existing.Evidence) == 0 && len(normalized.Evidence) > 0 {
			existing.Evidence = normalized.Evidence
		}
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

// negationConfidenceCap mirrors docs/math-evolution.md §4.2: r_i⁻ is capped at
// 0.7 so the math layer always has room to disagree.
const negationConfidenceCap = 0.7

// normalizeNegated filters the analyzer's negated_tags output:
//   - tag names must be in the supplied taxonomy (no inventing negations)
//   - duplicate tag names collapse to the highest confidence
//   - confidence is clamped to [0, negationConfidenceCap]
//   - entries with no evidence are dropped (negative signal requires textual
//     grounding; the verifier rejects unevidenced negations regardless)
func normalizeNegated(negated []NegatedTag, taxonomy []Tag) []NegatedTag {
	if len(negated) == 0 {
		return nil
	}
	taxonomyNames := make(map[string]string, len(taxonomy))
	for _, tag := range taxonomy {
		name := normalizeTagName(tag.Name)
		if name == "" {
			continue
		}
		taxonomyNames[name] = tag.Name
	}

	merged := make(map[string]NegatedTag, len(negated))
	for _, item := range negated {
		name := normalizeTagName(item.Tag)
		if name == "" {
			continue
		}
		exactName, ok := taxonomyNames[name]
		if !ok {
			continue
		}
		evidence := make([]string, 0, len(item.Evidence))
		for _, quote := range item.Evidence {
			quote = strings.TrimSpace(quote)
			if quote != "" {
				evidence = append(evidence, quote)
			}
		}
		if len(evidence) == 0 {
			continue
		}
		confidence := minFloat(negationConfidenceCap, maxFloat(0, item.Confidence))
		confidence = math.Round(confidence*100) / 100

		entry := NegatedTag{
			Tag:        exactName,
			Confidence: confidence,
			Evidence:   evidence,
		}
		existing, seen := merged[strings.ToLower(exactName)]
		if !seen || entry.Confidence > existing.Confidence {
			merged[strings.ToLower(exactName)] = entry
			continue
		}
		if len(existing.Evidence) == 0 && len(entry.Evidence) > 0 {
			existing.Evidence = entry.Evidence
			merged[strings.ToLower(exactName)] = existing
		}
	}

	out := make([]NegatedTag, 0, len(merged))
	for _, entry := range merged {
		out = append(out, entry)
	}
	slices.SortStableFunc(out, func(a, b NegatedTag) int {
		if c := cmp.Compare(b.Confidence, a.Confidence); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})
	return out
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
