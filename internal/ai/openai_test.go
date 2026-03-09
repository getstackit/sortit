package ai

import (
	"strings"
	"testing"
)

func TestBuildOpenAITaggingPromptIncludesGuidanceAndDescriptions(t *testing.T) {
	prompt := buildOpenAITaggingPrompt("Safari export hangs on iPad", []Tag{
		{Name: "export", Description: "download, file generation, sharing, or data extraction"},
		{Name: "safari", Description: "Safari or WebKit-specific behavior"},
	})

	if !strings.Contains(prompt, "Suggest at most 2 new tags") {
		t.Fatalf("expected prompt to mention suggested tag limit")
	}
	if !strings.Contains(prompt, "reuse these exact labels") {
		t.Fatalf("expected prompt to steer exact taxonomy reuse")
	}
	if !strings.Contains(prompt, "prefer a concrete application surface") {
		t.Fatalf("expected prompt to steer specific suggested tags")
	}
	if !strings.Contains(prompt, "export: download, file generation, sharing, or data extraction") {
		t.Fatalf("expected prompt to include taxonomy descriptions")
	}
}

func TestBuildOpenAITaggingSystemPromptAllowsConstrainedSuggestions(t *testing.T) {
	prompt := buildOpenAITaggingSystemPrompt()

	for _, expected := range []string{
		"Prefer supplied taxonomy tags whenever they reasonably fit.",
		"Prefer specific application surface tags over broad buckets like backend, frontend, or ui",
		"Suggest a new tag only when a materially important concept cannot be expressed well by the supplied taxonomy.",
		"Return at most 2 suggested tags",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected system prompt to contain %q", expected)
		}
	}
}
