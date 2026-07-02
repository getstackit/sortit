package matheval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// realEmbeddingsPath is the committed real-model fixture: text-embedding-3-small
// vectors for every fixture text (tags, issues, queries). It is generated once
// by `go run ./internal/matheval/cmd/embedfixture` (which needs OPENAI_API_KEY)
// and committed, so ordinary `go test` runs use it with no network and no key.
const realEmbeddingsPath = "testdata/real_embeddings.json"

// RealEmbeddings holds real text-embedding-3-small vectors for the fixture
// texts, keyed by the same IDs/names the synthetic corpus uses. The judgments,
// tag scores, and raw texts transfer unchanged; only the geometry is real.
//
// This is layer 1 of WP-301: real-model embeddings over the *existing* fixture
// texts. The texts themselves were generated from tag structure, so even real
// embeddings of them are one step removed from a real corpus (layer 2). It
// still retires the "embeddings are literally tag sums" floor argument — the
// only geometry here is whatever the production model puts on real sentences.
type RealEmbeddings struct {
	Model       string               `json:"model"`
	GeneratedAt string               `json:"generatedAt"`
	Dim         int                  `json:"dim"`
	Tags        map[string][]float64 `json:"tags"`
	Issues      map[string][]float64 `json:"issues"`
	Queries     map[string][]float64 `json:"queries"`
}

// LoadRealEmbeddings reads and lightly validates the committed real-embedding
// fixture file.
func LoadRealEmbeddings(path string) (RealEmbeddings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RealEmbeddings{}, err
	}
	var re RealEmbeddings
	if err := json.Unmarshal(data, &re); err != nil {
		return RealEmbeddings{}, fmt.Errorf("parse real embeddings %s: %w", path, err)
	}
	if re.Dim <= 0 {
		return RealEmbeddings{}, fmt.Errorf("real embeddings %s: dim must be positive, got %d", path, re.Dim)
	}
	return re, nil
}

// TagDescriptor is the text embedded for a tag. It mirrors production's
// tags.CatalogService.embedTag ("name - description"), so the real fixture's
// tag geometry matches how tags are embedded at runtime.
func TagDescriptor(name, description string) string {
	descriptor := name
	if d := strings.TrimSpace(description); d != "" {
		descriptor += " - " + d
	}
	return descriptor
}

// RealCorpus overlays the committed real-model vectors onto this (synthetic)
// corpus: same tags, issues, queries, tag scores, velocities, and raw texts —
// only the embeddings and the dimension change. Every fixture text must have a
// real vector of the declared dimension, so a stale or partial fixture fails
// loudly rather than silently mixing geometries.
func (c Corpus) RealCorpus(re RealEmbeddings) (Corpus, error) {
	out := Corpus{
		Dim:     re.Dim,
		Tags:    make([]CorpusTag, len(c.Tags)),
		Issues:  make([]CorpusIssue, len(c.Issues)),
		Queries: make([]CorpusQuery, len(c.Queries)),
	}

	for i, tag := range c.Tags {
		vec, ok := re.Tags[tag.Name]
		if !ok {
			return Corpus{}, fmt.Errorf("real embeddings missing tag %q", tag.Name)
		}
		out.Tags[i] = CorpusTag{
			Name:        tag.Name,
			Description: tag.Description,
			Specificity: tag.Specificity,
			Embedding:   append([]float64(nil), vec...),
		}
	}
	for i, issue := range c.Issues {
		vec, ok := re.Issues[issue.ID]
		if !ok {
			return Corpus{}, fmt.Errorf("real embeddings missing issue %q", issue.ID)
		}
		out.Issues[i] = CorpusIssue{
			ID:        issue.ID,
			Raw:       issue.Raw,
			TagScores: issue.TagScores,
			Velocity:  issue.Velocity,
			Embedding: append([]float64(nil), vec...),
		}
	}
	for i, q := range c.Queries {
		vec, ok := re.Queries[q.ID]
		if !ok {
			return Corpus{}, fmt.Errorf("real embeddings missing query %q", q.ID)
		}
		out.Queries[i] = CorpusQuery{
			ID:        q.ID,
			Raw:       q.Raw,
			Tags:      q.Tags,
			Embedding: append([]float64(nil), vec...),
		}
	}

	if err := out.validate(); err != nil {
		return Corpus{}, fmt.Errorf("real corpus invalid: %w", err)
	}
	return out, nil
}
