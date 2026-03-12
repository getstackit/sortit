package ai

import (
	"net/http"
	"strings"
	"testing"
)

func TestBuildOpenAITaggingPromptIncludesGuidanceAndDescriptions(t *testing.T) {
	prompt := buildOpenAITaggingPrompt("Safari export hangs on iPad", []Tag{
		{Name: "export", Description: "download, file generation, sharing, or data extraction workflows. Excludes general browsing unless data leaves the product."},
		{Name: "safari", Description: "Safari or WebKit-specific behavior"},
	})

	if !strings.Contains(prompt, "important residual concept remains") {
		t.Fatalf("expected prompt to mention residual concept test")
	}
	if !strings.Contains(prompt, "reuse these exact labels") {
		t.Fatalf("expected prompt to steer exact taxonomy reuse")
	}
	if !strings.Contains(prompt, "concrete reusable surface, subsystem, workflow, or artifact") {
		t.Fatalf("expected prompt to steer specific suggested tags")
	}
	if !strings.Contains(prompt, "already implied by a combination of existing tags") {
		t.Fatalf("expected prompt to reject correlated suggestions")
	}
	if !strings.Contains(prompt, "kind, failure mode, surface, platform, experience, and implementation layer") {
		t.Fatalf("expected prompt to mention taxonomy lanes")
	}
	if !strings.Contains(prompt, "export: download, file generation, sharing, or data extraction workflows. Excludes general browsing unless data leaves the product.") {
		t.Fatalf("expected prompt to include taxonomy descriptions")
	}
}

func TestBuildOpenAITaggingSystemPromptAllowsConstrainedSuggestions(t *testing.T) {
	prompt := buildOpenAITaggingSystemPrompt()

	for _, expected := range []string{
		"Use supplied taxonomy tags by default.",
		"Default to zero suggested tags.",
		"The taxonomy may include multiple axes such as issue kind, failure mode, affected surface, platform, experience, and implementation layer.",
		"Use the best supported tags across those axes instead of inventing cross-product tags.",
		"residual concept is not already expressed by any combination of 1 to 3 existing tags.",
		"orthogonal to the existing taxonomy rather than a predictable co-occurrence.",
		"Relevance must be between 0 and 1 inclusive, using up to 2 decimal places.",
		"Return at most 1 suggested tag unless 2 independent missing concepts are both central.",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected system prompt to contain %q", expected)
		}
	}
}

func TestOpenAIModelDefaults(t *testing.T) {
	cfg := OpenAIConfig{
		APIKey:     "test-key",
		HTTPClient: http.DefaultClient,
	}

	tagger, err := NewOpenAITagger(cfg)
	if err != nil {
		t.Fatalf("NewOpenAITagger returned error: %v", err)
	}
	if tagger.Model() != defaultOpenAITagModel {
		t.Fatalf("expected default tag model %q, got %q", defaultOpenAITagModel, tagger.Model())
	}

	canonicalizer, err := NewOpenAICanonicalizer(cfg)
	if err != nil {
		t.Fatalf("NewOpenAICanonicalizer returned error: %v", err)
	}
	if canonicalizer.model != defaultOpenAICanonicalModel {
		t.Fatalf("expected default canonical model %q, got %q", defaultOpenAICanonicalModel, canonicalizer.model)
	}

	embedder, err := NewOpenAIEmbedder(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIEmbedder returned error: %v", err)
	}
	if embedder.Model() != defaultOpenAIEmbeddingModel {
		t.Fatalf("expected default embedding model %q, got %q", defaultOpenAIEmbeddingModel, embedder.Model())
	}
}

func TestOpenAIModelOverrides(t *testing.T) {
	cfg := OpenAIConfig{
		APIKey:         "test-key",
		TagModel:       "gpt-test-tag",
		CanonicalModel: "gpt-test-canonical",
		EmbeddingModel: "text-embedding-test",
		HTTPClient:     http.DefaultClient,
	}

	tagger, err := NewOpenAITagger(cfg)
	if err != nil {
		t.Fatalf("NewOpenAITagger returned error: %v", err)
	}
	if tagger.Model() != cfg.TagModel {
		t.Fatalf("expected tag model %q, got %q", cfg.TagModel, tagger.Model())
	}

	canonicalizer, err := NewOpenAICanonicalizer(cfg)
	if err != nil {
		t.Fatalf("NewOpenAICanonicalizer returned error: %v", err)
	}
	if canonicalizer.model != cfg.CanonicalModel {
		t.Fatalf("expected canonical model %q, got %q", cfg.CanonicalModel, canonicalizer.model)
	}

	embedder, err := NewOpenAIEmbedder(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIEmbedder returned error: %v", err)
	}
	if embedder.Model() != cfg.EmbeddingModel {
		t.Fatalf("expected embedding model %q, got %q", cfg.EmbeddingModel, embedder.Model())
	}
}
