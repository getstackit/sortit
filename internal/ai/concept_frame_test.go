package ai

import (
	"context"
	"strings"
	"testing"
)

func TestStubProposeConceptFromCluster(t *testing.T) {
	stub := NewStubConceptProfiler()

	name, profile, err := stub.ProposeConceptFromCluster(
		context.Background(),
		[]string{"Safari PDF export hangs on large files", "Export to PDF times out"},
		ConceptFrame{},
	)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	// Deterministic: name derives from the first summary's leading words.
	if name != "safari pdf export" {
		t.Fatalf("unexpected stub name: %q", name)
	}
	if !strings.Contains(profile, "Safari PDF export hangs on large files") {
		t.Fatalf("expected the profile to cite the cluster, got %q", profile)
	}

	// An empty cluster yields an empty name, signaling the caller to skip it.
	emptyName, _, err := stub.ProposeConceptFromCluster(context.Background(), []string{"  ", ""}, ConceptFrame{})
	if err != nil {
		t.Fatalf("propose empty: %v", err)
	}
	if emptyName != "" {
		t.Fatalf("expected empty name for an empty cluster, got %q", emptyName)
	}
}

func TestBuildOpenAIClusterConceptSystemPromptFrames(t *testing.T) {
	bare := buildOpenAIClusterConceptSystemPrompt(ConceptFrame{})
	if strings.Contains(bare, "You are tagging issues for the following project") {
		t.Fatal("empty frame must not add grounding to the cluster prompt")
	}
	if !strings.Contains(bare, "Name that shared concept and profile it.") {
		t.Fatal("cluster prompt should always instruct naming + profiling")
	}

	framed := buildOpenAIClusterConceptSystemPrompt(ConceptFrame{Overview: "Sortit is an issue tracker"})
	if !strings.HasPrefix(framed, "You are tagging issues for the following project.") {
		t.Fatal("a non-empty frame should prime cluster naming with project grounding")
	}
	if !strings.HasSuffix(framed, bare) {
		t.Fatal("framed cluster prompt must be the preamble plus the unchanged base prompt")
	}
}

func TestConceptFrameIsEmpty(t *testing.T) {
	cases := []struct {
		name  string
		frame ConceptFrame
		want  bool
	}{
		{name: "zero value", frame: ConceptFrame{}, want: true},
		{name: "blank overview only whitespace", frame: ConceptFrame{Overview: "   \n\t"}, want: true},
		{name: "overview set", frame: ConceptFrame{Overview: "Sortit is an issue tracker."}, want: false},
		{name: "concepts only", frame: ConceptFrame{Concepts: []ConceptDigest{{SubjectTag: "ridge regression", Profile: "the ranking model"}}}, want: false},
		{
			name:  "both set",
			frame: ConceptFrame{Overview: "Sortit", Concepts: []ConceptDigest{{SubjectTag: "ridge regression"}}},
			want:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.frame.IsEmpty(); got != tc.want {
				t.Fatalf("IsEmpty() = %v, want %v", got, tc.want)
			}
		})
	}
}
