package ai

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestBuildOpenAITaggingPromptIncludesGuidanceAndDescriptions(t *testing.T) {
	prompt := buildOpenAITaggingPrompt("Safari export hangs on iPad", []Tag{
		{Name: "export", Description: "download, file generation, sharing, or data extraction workflows. Excludes general browsing unless data leaves the product."},
		{Name: "safari", Description: "Safari or WebKit-specific behavior"},
	}, nil)

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
	if !strings.Contains(prompt, "Examples are reference patterns, not templates.") {
		t.Fatalf("expected prompt to warn against copying example-only tags")
	}
	if !strings.Contains(prompt, "Prefer a compact tag set.") {
		t.Fatalf("expected prompt to prefer compact tag sets")
	}
	if !strings.Contains(prompt, "kind, failure mode, surface, platform, experience, and implementation layer") {
		t.Fatalf("expected prompt to mention taxonomy lanes")
	}
	if !strings.Contains(prompt, "export: download, file generation, sharing, or data extraction workflows. Excludes general browsing unless data leaves the product.") {
		t.Fatalf("expected prompt to include taxonomy descriptions")
	}
}

func TestBuildOpenAITaggingPromptIncludesHintTags(t *testing.T) {
	prompt := buildOpenAITaggingPrompt("Convert JSONB to pgvector", []Tag{
		{Name: "database", Description: "database-related work"},
		{Name: "cleanup", Description: "cleanup and tech debt"},
		{Name: "suggested-database-migration", Description: "schema migration work", Hint: true},
	}, nil)

	if !strings.Contains(prompt, "High-affinity tags") {
		t.Fatalf("expected prompt to include high-affinity tags section")
	}
	if !strings.Contains(prompt, "suggested-database-migration") {
		t.Fatalf("expected prompt to include hint tag name")
	}
}

func TestBuildOpenAITaggingPromptOmitsHintSectionWhenNoHints(t *testing.T) {
	prompt := buildOpenAITaggingPrompt("Safari export hangs on iPad", []Tag{
		{Name: "export"},
		{Name: "safari"},
	}, nil)

	if strings.Contains(prompt, "High-affinity tags") {
		t.Fatalf("expected no high-affinity tags section when no hints are present")
	}
}

func TestBuildOpenAITaggingPromptIncludesFewShotExamples(t *testing.T) {
	examples := []FewShotExample{
		{
			Text: "Search box clears after typing second character",
			Tags: []FewShotTag{
				{Name: "bug", Relevance: 0.95},
				{Name: "search", Relevance: 0.90},
			},
		},
	}
	prompt := buildOpenAITaggingPrompt("some issue text", []Tag{
		{Name: "bug"},
	}, examples)

	if !strings.Contains(prompt, "well-tagged issues for reference") {
		t.Fatal("expected prompt to include examples section header")
	}
	if !strings.Contains(prompt, "Example 1:") {
		t.Fatal("expected prompt to include example numbering")
	}
	if !strings.Contains(prompt, "Search box clears after typing second character") {
		t.Fatal("expected prompt to include example text")
	}
	if !strings.Contains(prompt, "bug (0.95)") {
		t.Fatal("expected prompt to include example tag with relevance")
	}
	if !strings.Contains(prompt, "search (0.90)") {
		t.Fatal("expected prompt to include second example tag")
	}
}

func TestBuildOpenAITaggingPromptOmitsExamplesSectionWhenEmpty(t *testing.T) {
	prompt := buildOpenAITaggingPrompt("some issue text", []Tag{
		{Name: "bug"},
	}, nil)

	if strings.Contains(prompt, "well-tagged issues for reference") {
		t.Fatal("expected no examples section when no examples provided")
	}
}

func TestBuildOpenAITaggingSystemPromptAllowsConstrainedSuggestions(t *testing.T) {
	prompt := buildOpenAITaggingSystemPrompt(ConceptFrame{})

	for _, expected := range []string{
		"Use supplied taxonomy tags by default.",
		"aiming for the most specific and descriptive set.",
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

func TestBuildOpenAITaggingSystemPromptConceptFrame(t *testing.T) {
	// An empty frame renders no grounding — the prompt is byte-identical to the
	// bare prompt, guarding the fresh-install no-op.
	if pre := conceptFramePreamble(ConceptFrame{}); pre != "" {
		t.Fatalf("empty frame should render no preamble, got %q", pre)
	}
	base := buildOpenAITaggingSystemPrompt(ConceptFrame{})
	if strings.Contains(base, "You are tagging issues for the following project") {
		t.Fatal("empty frame must not add grounding to the prompt")
	}

	frame := ConceptFrame{
		Overview: "Sortit is an issue tracker built around a factor model over tag relevance",
		Concepts: []ConceptDigest{
			{SubjectTag: "ridge regression", Profile: "the diagonal-penalty ranking model"},
			{SubjectTag: "factor model", Profile: "low-rank decomposition of tag relevance"},
		},
	}
	grounded := buildOpenAITaggingSystemPrompt(frame)

	for _, expected := range []string{
		"You are tagging issues for the following project.",
		"Project: Sortit is an issue tracker built around a factor model over tag relevance.",
		"This project's core concepts (its own vocabulary):",
		"- ridge regression — the diagonal-penalty ranking model",
		"- factor model — low-rank decomposition of tag relevance",
		"assign that concept's tag rather than a generic substitute.",
		"don't attach an unrelated project noun.",
	} {
		if !strings.Contains(grounded, expected) {
			t.Fatalf("expected grounded prompt to contain %q", expected)
		}
	}

	// The grounding is the prefix (largest cacheable prefix), and the base
	// classification instructions follow byte-for-byte unchanged.
	if !strings.HasPrefix(grounded, "You are tagging issues for the following project.") {
		t.Fatal("frame grounding must prefix the system prompt")
	}
	if !strings.HasSuffix(grounded, base) {
		t.Fatal("grounded prompt must be preamble + the unchanged base prompt")
	}
}

func TestBuildOpenAITaggingPromptsRequestOptionalEvidence(t *testing.T) {
	systemPrompt := buildOpenAITaggingSystemPrompt(ConceptFrame{})
	for _, expected := range []string{
		"may also include an evidence array",
		"verbatim quotes copied character-for-character from the issue text",
		"Tags with evidence citations are treated as higher confidence.",
	} {
		if !strings.Contains(systemPrompt, expected) {
			t.Fatalf("expected system prompt to contain %q", expected)
		}
	}
	if strings.Contains(systemPrompt, "do not include that tag") {
		t.Fatal("system prompt should not reject tags without evidence")
	}

	userPrompt := buildOpenAITaggingPrompt("an issue", []Tag{{Name: "bug"}}, nil)
	if !strings.Contains(userPrompt, "include an evidence array") {
		t.Fatalf("expected user prompt to mention evidence citations")
	}
	if strings.Contains(userPrompt, "do not assign the tag") {
		t.Fatal("user prompt should not reject tags without evidence")
	}
}

func TestBuildOpenAITaggingPromptInstructsNegatedTags(t *testing.T) {
	systemPrompt := buildOpenAITaggingSystemPrompt(ConceptFrame{})
	for _, expected := range []string{
		"negated_tags",
		"EXPLICITLY REFUTES",
		"Evidence for negated tags is required",
		"Never invent tag names in negated_tags",
	} {
		if !strings.Contains(systemPrompt, expected) {
			t.Fatalf("expected system prompt to contain %q", expected)
		}
	}

	userPrompt := buildOpenAITaggingPrompt("an issue", []Tag{{Name: "bug"}}, nil)
	if !strings.Contains(userPrompt, "negated_tags") {
		t.Fatalf("expected user prompt to mention negated_tags")
	}
	if !strings.Contains(userPrompt, "EXPLICITLY REFUTES") {
		t.Fatalf("expected user prompt to instruct on explicit refutation")
	}
}

func TestOpenAITagScoresResponseAcceptsMissingNegatedTags(t *testing.T) {
	body := `{"tags": [{"tag": "bug", "relevance": 0.9}]}`
	var payload openAITagScoresResponse
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(payload.Tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(payload.Tags))
	}
	if payload.NegatedTags != nil {
		t.Fatalf("expected NegatedTags to be nil when absent, got %+v", payload.NegatedTags)
	}
}

func TestOpenAITagScoresResponseParsesNegatedTags(t *testing.T) {
	body := `{
		"tags": [{"tag": "bug", "relevance": 0.9}],
		"negated_tags": [
			{"tag": "regression", "confidence": 0.85, "evidence": ["this is not a regression"]}
		]
	}`
	var payload openAITagScoresResponse
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(payload.NegatedTags) != 1 {
		t.Fatalf("expected 1 negated tag, got %d", len(payload.NegatedTags))
	}
	got := payload.NegatedTags[0]
	if got.Tag != "regression" || got.Confidence != 0.85 || len(got.Evidence) != 1 {
		t.Fatalf("unexpected negated tag payload: %+v", got)
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
