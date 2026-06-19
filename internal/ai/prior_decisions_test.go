package ai

import (
	"strings"
	"testing"
)

func TestBuildOpenAITaggingPromptIncludesPriorDecisions(t *testing.T) {
	prompt := buildOpenAITaggingPrompt(
		"Should issue search use ridge or rank-1?",
		[]Tag{{Name: "search"}},
		nil,
		PriorDecision{
			Title:   "Ridge is the default",
			Summary: "Issue search defaults to the ridge similarity model.",
			Tags:    []string{"search"},
		},
	)

	if !strings.Contains(prompt, "Documented prior decisions") {
		t.Fatal("expected prompt to include the prior-decisions section header")
	}
	if !strings.Contains(prompt, "Ridge is the default") {
		t.Fatal("expected prompt to include the decision title")
	}
	if !strings.Contains(prompt, "ridge similarity model") {
		t.Fatal("expected prompt to include the decision summary")
	}
	if !strings.Contains(prompt, "[search]") {
		t.Fatal("expected prompt to include the decision's anchor tags")
	}
}

func TestBuildOpenAITaggingPromptOmitsPriorDecisionsWhenNone(t *testing.T) {
	prompt := buildOpenAITaggingPrompt("an issue", []Tag{{Name: "search"}}, nil)
	if strings.Contains(prompt, "Documented prior decisions") {
		t.Fatal("expected no prior-decisions section when none are supplied")
	}
}
