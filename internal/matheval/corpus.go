// Package matheval is an offline evaluation harness for Sortit's scoring and
// ranking math. It holds a synthetic-but-realistic fixture corpus (issues,
// tags, and queries with deterministic embeddings checked into testdata/), a
// hand-labeled judgment set, and ranking/decomposition metrics. The harness
// runs as a normal `go test` and compares results against a golden baseline,
// so any change to internal/scoring/constants.go or the math packages is
// measurable instead of hand-felt (whitepaper §10.2, item 9).
//
// No network or AI services are involved: embeddings are small deterministic
// vectors synthesized from FNV hashes at corpus-generation time.
package matheval

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"

	"sortit/internal/issues"
)

// EmbeddingDim is the dimensionality of all fixture embeddings. Small enough
// to keep testdata readable, large enough that unrelated hash vectors are
// close to orthogonal.
const EmbeddingDim = 24

// Corpus is the fixture dataset persisted in testdata/corpus.json.
type Corpus struct {
	Dim     int           `json:"dim"`
	Tags    []CorpusTag   `json:"tags"`
	Issues  []CorpusIssue `json:"issues"`
	Queries []CorpusQuery `json:"queries"`
}

// CorpusTag mirrors the fields of issues.Tag the math consumes.
type CorpusTag struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Specificity float64   `json:"specificity"`
	Embedding   []float64 `json:"embedding"`
}

// CorpusIssue is a synthetic issue with pre-computed tag scores and embedding,
// standing in for the output of the AI analyzer + embedding service.
type CorpusIssue struct {
	ID        string     `json:"id"`
	Raw       string     `json:"raw"`
	TagScores []TagScore `json:"tagScores,omitempty"`
	Velocity  *float64   `json:"velocity,omitempty"`
	Embedding []float64  `json:"embedding"`
}

// CorpusQuery is a search query with analyzer tag scores and embedding, as
// the search endpoint would receive after query enrichment.
type CorpusQuery struct {
	ID        string     `json:"id"`
	Raw       string     `json:"raw"`
	Tags      []TagScore `json:"tags,omitempty"`
	Embedding []float64  `json:"embedding"`
}

// TagScore is a (tag, relevance) pair.
type TagScore struct {
	Tag       string  `json:"tag"`
	Relevance float64 `json:"relevance"`
}

// LoadCorpus reads and validates a corpus fixture file.
func LoadCorpus(path string) (Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Corpus{}, err
	}
	var c Corpus
	if err := json.Unmarshal(data, &c); err != nil {
		return Corpus{}, fmt.Errorf("parse corpus %s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return Corpus{}, fmt.Errorf("invalid corpus %s: %w", path, err)
	}
	return c, nil
}

func (c Corpus) validate() error {
	if c.Dim <= 0 {
		return fmt.Errorf("dim must be positive, got %d", c.Dim)
	}
	tagNames := make(map[string]struct{}, len(c.Tags))
	for _, tag := range c.Tags {
		if len(tag.Embedding) != c.Dim {
			return fmt.Errorf("tag %q embedding has dim %d, want %d", tag.Name, len(tag.Embedding), c.Dim)
		}
		tagNames[tag.Name] = struct{}{}
	}
	seenIssues := make(map[string]struct{}, len(c.Issues))
	for _, issue := range c.Issues {
		if _, dup := seenIssues[issue.ID]; dup {
			return fmt.Errorf("duplicate issue ID %q", issue.ID)
		}
		seenIssues[issue.ID] = struct{}{}
		if len(issue.Embedding) != c.Dim {
			return fmt.Errorf("issue %q embedding has dim %d, want %d", issue.ID, len(issue.Embedding), c.Dim)
		}
		for _, ts := range issue.TagScores {
			if _, ok := tagNames[ts.Tag]; !ok {
				return fmt.Errorf("issue %q references unknown tag %q", issue.ID, ts.Tag)
			}
		}
	}
	seenQueries := make(map[string]struct{}, len(c.Queries))
	for _, q := range c.Queries {
		if _, dup := seenQueries[q.ID]; dup {
			return fmt.Errorf("duplicate query ID %q", q.ID)
		}
		seenQueries[q.ID] = struct{}{}
		if len(q.Embedding) != c.Dim {
			return fmt.Errorf("query %q embedding has dim %d, want %d", q.ID, len(q.Embedding), c.Dim)
		}
		for _, ts := range q.Tags {
			if _, ok := tagNames[ts.Tag]; !ok {
				return fmt.Errorf("query %q references unknown tag %q", q.ID, ts.Tag)
			}
		}
	}
	return nil
}

// fixtureAge keeps every fixture issue the same age so the freshness
// multiplier is uniform and rankings stay deterministic regardless of when
// the harness runs.
const fixtureAge = 30 * 24 * time.Hour

// StoreIssues converts the corpus into store-shaped issues for the search and
// map entry points.
func (c Corpus) StoreIssues(now time.Time) []issues.Issue {
	out := make([]issues.Issue, len(c.Issues))
	for i, fixture := range c.Issues {
		issue := issues.Issue{
			ID:        fixture.ID,
			Raw:       fixture.Raw,
			Status:    issues.StatusOpen,
			CreatedAt: now.Add(-fixtureAge),
			TagScores: toTagRelevances(fixture.TagScores),
			Embedding: append([]float64(nil), fixture.Embedding...),
		}
		if fixture.Velocity != nil {
			velocity := *fixture.Velocity
			issue.LifecycleMetrics = &issues.IssueLifecycleMetrics{Velocity: &velocity}
		}
		out[i] = issue
	}
	return out
}

// StoreTags converts the corpus tags into store-shaped tags.
func (c Corpus) StoreTags() []issues.Tag {
	out := make([]issues.Tag, len(c.Tags))
	for i, fixture := range c.Tags {
		specificity := fixture.Specificity
		out[i] = issues.Tag{
			Name:        fixture.Name,
			Description: fixture.Description,
			Embedding:   append([]float64(nil), fixture.Embedding...),
			Specificity: &specificity,
		}
	}
	return out
}

// TagNames returns the sorted tag name list, matching the ordering the
// runtime derives in runtimeMapInputs.
func (c Corpus) TagNames() []string {
	names := make([]string, len(c.Tags))
	for i, tag := range c.Tags {
		names[i] = tag.Name
	}
	slices.Sort(names)
	return names
}

// TagEmbeddings returns the tag name → embedding map.
func (c Corpus) TagEmbeddings() map[string][]float64 {
	out := make(map[string][]float64, len(c.Tags))
	for _, tag := range c.Tags {
		out[tag.Name] = append([]float64(nil), tag.Embedding...)
	}
	return out
}

// IssueEmbeddings returns the issue ID → embedding map.
func (c Corpus) IssueEmbeddings() map[string][]float64 {
	out := make(map[string][]float64, len(c.Issues))
	for _, issue := range c.Issues {
		out[issue.ID] = append([]float64(nil), issue.Embedding...)
	}
	return out
}

// QueryByID returns the query fixture with the given ID.
func (c Corpus) QueryByID(id string) (CorpusQuery, bool) {
	for _, q := range c.Queries {
		if q.ID == id {
			return q, true
		}
	}
	return CorpusQuery{}, false
}

func toTagRelevances(scores []TagScore) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}
	out := make([]issues.TagRelevance, len(scores))
	for i, ts := range scores {
		out[i] = issues.TagRelevance{Tag: ts.Tag, Relevance: ts.Relevance}
	}
	return out
}
