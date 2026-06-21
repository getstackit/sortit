package ai

import (
	"context"
	"strings"
)

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

// PriorDecision is a documented memory surfaced to the tagger as context, so a
// new issue can be tagged consistently with decisions already made and so the
// model is aware the question may already be settled ("have we decided this
// before?").
type PriorDecision struct {
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Tags    []string `json:"tags,omitempty"`
}

// ConceptDigest is one curated concept condensed for the tagging frame: the
// subject tag (the project's own noun) and a short profile of what it means.
type ConceptDigest struct {
	SubjectTag string `json:"subjectTag"`
	Profile    string `json:"profile"`
}

// ConceptFrame is the project's stable identity, surfaced to the tagger as
// grounding so every issue is tagged within the project's vocabulary rather than
// from the issue text and its retrieval neighborhood alone. Overview states what
// the project is; Concepts are its curated core nouns. It is assembled once per
// enrichment batch from the memory store (the active overview + active concepts)
// and forms the stable, cacheable prefix of the tagging system prompt.
type ConceptFrame struct {
	Overview string          `json:"overview"`
	Concepts []ConceptDigest `json:"concepts,omitempty"`
}

// IsEmpty reports whether the frame carries no grounding — a blank overview and
// no concepts. When empty the tagging prompt renders exactly as before, so a
// fresh install with neither set is a no-op.
func (f ConceptFrame) IsEmpty() bool {
	return strings.TrimSpace(f.Overview) == "" && len(f.Concepts) == 0
}

type Tagger interface {
	Score(ctx context.Context, text string, tags []Tag, examples []FewShotExample, priorDecisions []PriorDecision, frame ConceptFrame) (ScoreResult, error)
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

// ConceptProfiler generates the canonical prose profile of a single concept (a
// subsystem, component, or domain concept) from the tag it is named for and a
// sample of issues that reference it.
type ConceptProfiler interface {
	GenerateConceptProfile(ctx context.Context, tag string, issueSummaries []string) (string, error)
	// ProposeConceptFromCluster names and profiles a NEW concept from a cluster of
	// issues that share an unexplained embedding residual — the residual-mining
	// counterpart to GenerateConceptProfile, which profiles an *existing* tag. The
	// frame primes naming with the project's vocabulary so the invented subject tag
	// fits the project's own naming. It returns the proposed subject tag (a short,
	// lowercase, reusable noun) and its profile prose.
	ProposeConceptFromCluster(ctx context.Context, issueSummaries []string, frame ConceptFrame) (name string, profile string, err error)
}
