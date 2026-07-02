package ai

import (
	"context"
	"testing"
)

func TestStubThemeLabelerJoinsTopTags(t *testing.T) {
	labeler := NewStubThemeLabeler()
	name, description, err := labeler.ProposeThemeLabel(context.Background(), []ThemeLabelTag{
		{Tag: "mobile", Loading: 0.6},
		{Tag: "ui", Loading: 0.3},
		{Tag: "ios", Loading: 0.1},
		{Tag: "gestures", Loading: 0.05}, // beyond the cap — dropped
	}, nil, ConceptFrame{})
	if err != nil {
		t.Fatalf("ProposeThemeLabel: %v", err)
	}
	if name != "mobile / ui / ios" {
		t.Fatalf("name = %q, want %q", name, "mobile / ui / ios")
	}
	if description == "" {
		t.Fatal("expected a non-empty description")
	}
}

func TestStubThemeLabelerEmptyTagsYieldsEmptyName(t *testing.T) {
	labeler := NewStubThemeLabeler()
	name, _, err := labeler.ProposeThemeLabel(context.Background(), []ThemeLabelTag{{Tag: "  "}}, nil, ConceptFrame{})
	if err != nil {
		t.Fatalf("ProposeThemeLabel: %v", err)
	}
	if name != "" {
		t.Fatalf("expected empty name for tagless input, got %q", name)
	}
}

func TestAnalyzerProposeThemeLabelPassthrough(t *testing.T) {
	analyzer := NewAnalyzer(NewStubTagger(), NewStubEmbedder())
	name, _, err := analyzer.ProposeThemeLabel(context.Background(), []ThemeLabelTag{{Tag: "billing"}}, nil, ConceptFrame{})
	if err != nil {
		t.Fatalf("ProposeThemeLabel: %v", err)
	}
	if name != "billing" {
		t.Fatalf("default analyzer should use the stub labeler, got name %q", name)
	}
}

func TestAnalyzerSetThemeLabelerSwapsProvider(t *testing.T) {
	analyzer := NewAnalyzer(NewStubTagger(), NewStubEmbedder())
	analyzer.SetThemeLabeler(fixedThemeLabeler{name: "custom", description: "custom desc"})
	name, description, err := analyzer.ProposeThemeLabel(context.Background(), []ThemeLabelTag{{Tag: "billing"}}, nil, ConceptFrame{})
	if err != nil {
		t.Fatalf("ProposeThemeLabel: %v", err)
	}
	if name != "custom" || description != "custom desc" {
		t.Fatalf("expected the swapped labeler's output, got (%q, %q)", name, description)
	}
}

type fixedThemeLabeler struct {
	name        string
	description string
}

func (l fixedThemeLabeler) ProposeThemeLabel(_ context.Context, _ []ThemeLabelTag, _ []string, _ ConceptFrame) (string, string, error) {
	return l.name, l.description, nil
}
