package ai

import "context"

type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
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

type IssueAnalysis struct {
	Tags      []TagScore    `json:"tags"`
	Embedding EmbeddingInfo `json:"embedding"`
	Tagger    ModelInfo     `json:"tagger"`
	Embedder  ModelInfo     `json:"embedder"`
}

type Tagger interface {
	Score(ctx context.Context, text string, tags []Tag) ([]TagScore, error)
	Provider() string
	Model() string
}

type Embedder interface {
	EmbedText(ctx context.Context, text string) (EmbeddingResult, error)
	Provider() string
	Model() string
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
