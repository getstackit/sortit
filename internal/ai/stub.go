package ai

import (
	"cmp"
	"context"
	"encoding/binary"
	"hash/fnv"
	"math"
	"slices"
	"strings"
)

const stubEmbeddingDimensions = 24

var stubTagSignals = map[string][]string{
	"bug":         {"bug", "error", "broken", "failure", "failing", "wrong"},
	"crash":       {"crash", "panic", "freeze", "frozen", "hang", "hung"},
	"feature":     {"feature", "support", "allow", "enable", "add"},
	"idea":        {"idea", "maybe", "could", "should", "consider"},
	"improvement": {"improve", "improvement", "better", "refine", "polish", "simplify"},
	"ui":          {"ui", "interface", "button", "layout", "modal", "dropdown"},
	"ux":          {"ux", "friction", "confusing", "flow", "discoverable", "intuitive"},
	"frontend":    {"frontend", "browser", "client", "react", "render", "hydration"},
	"performance": {"slow", "latency", "performance", "fast", "faster", "timeout"},
	"safari":      {"safari", "webkit", "ios", "iphone", "ipad"},
	"onboarding":  {"onboarding", "signup", "invite", "empty state", "first run", "tutorial"},
	"search":      {"search", "query", "filter", "results", "ranking"},
	"export":      {"export", "csv", "download", "pdf", "share"},
}

type StubTagger struct{}

func NewStubTagger() *StubTagger {
	return &StubTagger{}
}

func (t *StubTagger) Score(_ context.Context, text string, tags []Tag) ([]TagScore, error) {
	lower := strings.ToLower(text)
	scores := make([]TagScore, 0, len(tags))
	for _, tag := range tags {
		if tag.Name == "" {
			continue
		}

		score := 0.04 + stubNoise(text, tag.Name)*0.08
		tagName := strings.ToLower(tag.Name)
		score += matchWeight(lower, tagName, 0.34)
		for _, signal := range stubTagSignals[tagName] {
			score += matchWeight(lower, signal, 0.18)
		}
		if tag.Description != "" {
			score += matchWeight(lower, strings.ToLower(tag.Description), 0.08)
		}

		scores = append(scores, TagScore{
			Tag:       tag.Name,
			Relevance: minFloat(1, score),
		})
	}

	slices.SortStableFunc(scores, func(a, b TagScore) int {
		if c := cmp.Compare(b.Relevance, a.Relevance); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})
	return scores, nil
}

func (t *StubTagger) Provider() string {
	return "stub"
}

func (t *StubTagger) Model() string {
	return "keyword-v1"
}

type StubEmbedder struct {
	dimensions     int
	maxChunkTokens int
	previewSize    int
}

type StubCanonicalizer struct{}

func NewStubEmbedder() *StubEmbedder {
	return &StubEmbedder{
		dimensions:     stubEmbeddingDimensions,
		maxChunkTokens: defaultEmbeddingChunkTokens,
		previewSize:    defaultEmbeddingPreviewSize,
	}
}

func NewStubCanonicalizer() *StubCanonicalizer {
	return &StubCanonicalizer{}
}

func (e *StubEmbedder) EmbedText(ctx context.Context, text string) (EmbeddingResult, error) {
	return embedChunkedText(ctx, text, e.maxChunkTokens, e.previewSize, e.embedTexts)
}

func (e *StubEmbedder) embedTexts(_ context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vector := make([]float64, e.dimensions)
		tokens := strings.Fields(strings.ToLower(text))
		if len(tokens) == 0 {
			embeddings = append(embeddings, make([]float32, e.dimensions))
			continue
		}

		for idx, token := range tokens {
			bucket := int(hashUint32(token) % uint32(e.dimensions)) //nolint:gosec
			sign := 1.0
			if hashUint32(token+"-sign")%2 == 0 {
				sign = -1
			}

			vector[bucket] += sign * (1 + float64((idx%5))/10)
			nextBucket := int(hashUint32(token+"-next") % uint32(e.dimensions)) //nolint:gosec
			vector[nextBucket] += sign * 0.35
		}

		norm := 0.0
		for _, value := range vector {
			norm += value * value
		}
		norm = math.Sqrt(norm)
		embedding := make([]float32, e.dimensions)
		if norm == 0 {
			embeddings = append(embeddings, embedding)
			continue
		}

		for i, value := range vector {
			embedding[i] = float32(value / norm)
		}
		embeddings = append(embeddings, embedding)
	}
	return embeddings, nil
}

func (e *StubEmbedder) Provider() string {
	return "stub"
}

func (e *StubEmbedder) Model() string {
	return "hash-embedding-v1"
}

func (c *StubCanonicalizer) CanonicalizeDiscussion(_ context.Context, posts []string) (string, error) {
	normalized := make([]string, 0, len(posts))
	for _, post := range posts {
		post = strings.TrimSpace(post)
		if post == "" {
			continue
		}
		normalized = append(normalized, post)
	}
	if len(normalized) == 0 {
		return "", nil
	}
	if len(normalized) == 1 {
		return normalized[0], nil
	}

	var builder strings.Builder
	builder.WriteString(normalized[0])
	for _, post := range normalized[1:] {
		builder.WriteString("\n\nAdditional context: ")
		builder.WriteString(post)
	}
	return builder.String(), nil
}

func matchWeight(text string, signal string, weight float64) float64 {
	if signal == "" {
		return 0
	}
	count := strings.Count(text, signal)
	if count == 0 {
		return 0
	}
	return weight * math.Min(1, float64(count))
}

func stubNoise(text string, salt string) float64 {
	sum := hashUint32(text + "::" + salt)
	return float64(sum%1000) / 1000
}

func hashUint32(input string) uint32 {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(input))
	return binary.BigEndian.Uint32(hasher.Sum(nil))
}
