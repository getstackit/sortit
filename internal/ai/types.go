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
	// Evidence holds short verbatim quotes from the source text that justify
	// this tag. The model is instructed to copy fragments rather than
	// paraphrase so the verifier can confirm grounding.
	Evidence []string `json:"evidence,omitempty"`
}

// NegatedTag is a tag from the supplied taxonomy that the issue text
// explicitly refutes. Confidence is the tagger's strength of the negation
// claim in [0, 1]; the verifier later caps it at 0.7 when persisting.
// Evidence is required: a negated tag without evidence is dropped during
// normalization.
type NegatedTag struct {
	Tag        string   `json:"tag"`
	Confidence float64  `json:"confidence"`
	Evidence   []string `json:"evidence,omitempty"`
}

// ScoreResult is the structured response from Tagger.Score. Wrapping the
// return value in a struct lets later signals (e.g. confidence intervals
// or alternative candidate lists) be added without breaking callers.
type ScoreResult struct {
	Tags    []TagScore
	Negated []NegatedTag
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
	Negated   []NegatedTag
	Embedding EmbeddingResult
	Tagger    ModelInfo
	Embedder  ModelInfo
}

type IssueAnalysis struct {
	Tags      []TagScore    `json:"tags"`
	Negated   []NegatedTag  `json:"negated,omitempty"`
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
	Score(ctx context.Context, text string, tags []Tag, examples []FewShotExample) (ScoreResult, error)
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
