package ai

import "context"

type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Hint is set when embedding similarity suggests this tag is relevant.
	// The prompt uses this to draw the AI's attention to high-affinity tags.
	Hint bool `json:"-"`
}

type TagScore struct {
	Tag         string  `json:"tag"`
	Relevance   float64 `json:"relevance"`
	Suggested   bool    `json:"suggested,omitempty"`
	Description string  `json:"description,omitempty"`
}

type ModelInfo struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type EmbeddingInfo struct {
	Dimensions          int       `json:"dimensions"`
	Preview             []float32 `json:"preview"`
	ChunkCount          int       `json:"chunkCount"`
	EstimatedTokenCount int       `json:"estimatedTokenCount"`
	PooledFromChunks    bool      `json:"pooledFromChunks"`
}

type EmbeddingResult struct {
	Vector []float32
	Info   EmbeddingInfo
}

type AnalyzedIssue struct {
	Tags      []TagScore
	Embedding EmbeddingResult
	Tagger    ModelInfo
	Embedder  ModelInfo
}

type IssueAnalysis struct {
	Tags      []TagScore    `json:"tags"`
	Embedding EmbeddingInfo `json:"embedding"`
	Tagger    ModelInfo     `json:"tagger"`
	Embedder  ModelInfo     `json:"embedder"`
}

// FewShotExample is a curated tagging example included in the prompt so the
// model can see what correct, specific tagging looks like.
type FewShotExample struct {
	Text      string       `json:"text"`
	Tags      []FewShotTag `json:"tags"`
	Embedding []float64    `json:"-"`
}

// FewShotTag is a single tag assignment inside a few-shot example.
type FewShotTag struct {
	Name      string  `json:"name"`
	Relevance float64 `json:"relevance"`
}

type Tagger interface {
	Score(ctx context.Context, text string, tags []Tag, examples []FewShotExample) ([]TagScore, error)
	Provider() string
	Model() string
}

type Embedder interface {
	EmbedText(ctx context.Context, text string) (EmbeddingResult, error)
	Provider() string
	Model() string
}

type SpecificityScorer interface {
	ScoreSpecificity(ctx context.Context, tag Tag, catalog []Tag) (float64, error)
}

type Canonicalizer interface {
	CanonicalizeDiscussion(ctx context.Context, posts []string) (string, error)
}

func TagsFromNames(names []string) []Tag {
	tags := make([]Tag, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		tags = append(tags, Tag{Name: name})
	}
	return tags
}
