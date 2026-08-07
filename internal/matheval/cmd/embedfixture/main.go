// Command embedfixture generates the real-embedding fixture for the matheval
// harness (WP-301, layer 1). It embeds every fixture text — the 16 tag
// descriptors, 48 issue raw texts, and 32 query raw texts — with the production
// text-embedding-3-small model via the internal/ai OpenAI client, and writes
// testdata/real_embeddings.json. The output is committed so ordinary `go test
// ./internal/matheval/...` runs against real geometry with no network and no key.
//
// This is a one-off network tool, excluded from normal test runs. It needs
// OPENAI_API_KEY in the environment (or sourced from ./.env). The corpus is
// tiny (96 texts), so cost is negligible.
//
// Usage:
//
//	OPENAI_API_KEY=... go run ./internal/matheval/cmd/embedfixture \
//	  -out internal/matheval/testdata/real_embeddings.json -date 2026-07-02
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"sortit/internal/ai"
	"sortit/internal/matheval"
)

func main() {
	out := flag.String("out", "internal/matheval/testdata/real_embeddings.json", "output path for the real-embedding fixture")
	date := flag.String("date", time.Now().UTC().Format("2006-01-02"), "generatedAt date recorded in fixture metadata (YYYY-MM-DD)")
	model := flag.String("model", "", "embedding model override (defaults to the ai package's text-embedding-3-small)")
	flag.Parse()

	if err := run(*out, *date, *model); err != nil {
		log.Fatalf("embedfixture: %v", err)
	}
}

func run(outPath, date, modelOverride string) error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required (source ./.env if the shell did not inherit it)")
	}

	embedder, err := ai.NewOpenAIEmbedder(ai.OpenAIConfig{
		APIKey:         apiKey,
		EmbeddingModel: modelOverride,
	})
	if err != nil {
		return fmt.Errorf("build embedder: %w", err)
	}

	corpus := matheval.GenerateCorpus()
	ctx := context.Background()

	re := matheval.RealEmbeddings{
		Model:       embedder.Model(),
		GeneratedAt: date,
		Tags:        make(map[string][]float64, len(corpus.Tags)),
		Issues:      make(map[string][]float64, len(corpus.Issues)),
		Queries:     make(map[string][]float64, len(corpus.Queries)),
	}

	dim := 0
	embed := func(what, text string) ([]float64, error) {
		result, err := embedder.EmbedText(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed %s: %w", what, err)
		}
		vec := roundFloat32To64(result.Vector)
		if dim == 0 {
			dim = len(vec)
		} else if len(vec) != dim {
			return nil, fmt.Errorf("embed %s: dim %d, expected %d", what, len(vec), dim)
		}
		return vec, nil
	}

	// Tags: embed "name - description", matching production's embedTag.
	for _, tag := range corpus.Tags {
		vec, err := embed("tag "+tag.Name, matheval.TagDescriptor(tag.Name, tag.Description))
		if err != nil {
			return err
		}
		re.Tags[tag.Name] = vec
		log.Printf("embedded tag %s", tag.Name)
	}
	// Issues: embed the raw text, the same input production embeds.
	for _, issue := range corpus.Issues {
		vec, err := embed("issue "+issue.ID, issue.Raw)
		if err != nil {
			return err
		}
		re.Issues[issue.ID] = vec
		log.Printf("embedded issue %s", issue.ID)
	}
	// Queries: embed the raw query text.
	for _, q := range corpus.Queries {
		vec, err := embed("query "+q.ID, q.Raw)
		if err != nil {
			return err
		}
		re.Queries[q.ID] = vec
		log.Printf("embedded query %s", q.ID)
	}

	re.Dim = dim

	// Compact encoding (no indentation) keeps the ~96×1536-float file to ~1.3 MB.
	// Map keys marshal sorted, so the output is deterministic given fixed inputs.
	data, err := json.Marshal(re)
	if err != nil {
		return fmt.Errorf("marshal fixture: %w", err)
	}
	if err := os.WriteFile(outPath, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	log.Printf("wrote %s (%d tags, %d issues, %d queries, dim=%d, model=%s, %d bytes)",
		outPath, len(re.Tags), len(re.Issues), len(re.Queries), dim, re.Model, len(data))
	return nil
}

// roundFloat32To64 converts an OpenAI float32 vector to float64 rounded to 6
// decimals, matching the synthetic corpus's stored precision and keeping the
// committed JSON compact. The runtime re-normalizes stored embeddings, so the
// ~1e-6 rounding error is immaterial.
func roundFloat32To64(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = math.Round(float64(x)*1e6) / 1e6
	}
	return out
}
