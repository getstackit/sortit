package ai

import (
	"os"
	"testing"
)

func TestNewAnalyzerFromEnvUsesStubByDefaultInTests(t *testing.T) {
	t.Setenv("AI_PROVIDER", "")
	t.Setenv("OPENAI_API_KEY", "")

	analyzer, err := NewAnalyzerFromEnv()
	if err != nil {
		t.Fatalf("NewAnalyzerFromEnv returned error: %v", err)
	}
	if analyzer == nil {
		t.Fatal("expected analyzer")
	}
}

func TestNewAnalyzerFromEnvRejectsRealProviderDuringTests(t *testing.T) {
	if !runningUnderGoTest() {
		t.Fatal("expected to be running under go test")
	}

	t.Setenv("AI_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")

	analyzer, err := NewAnalyzerFromEnv()
	if err == nil {
		t.Fatal("expected error when configuring real provider under go test")
	}
	if analyzer != nil {
		t.Fatal("expected nil analyzer when configuring real provider under go test")
	}
}

func TestRunningUnderGoTest(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		t.Skip("helper process")
	}

	if !runningUnderGoTest() {
		t.Fatal("expected test flag to be present under go test")
	}
}

func TestCanonicalModelFallsBackToExplicitTagModel(t *testing.T) {
	t.Setenv("OPENAI_TAG_MODEL", "gpt-test-tag")
	t.Setenv("OPENAI_CANONICAL_MODEL", "")

	tagModel := os.Getenv("OPENAI_TAG_MODEL")
	canonicalModel := openAICanonicalModelFromEnv(tagModel)
	if canonicalModel != tagModel {
		t.Fatalf("expected canonical model fallback %q, got %q", tagModel, canonicalModel)
	}
}

func TestCanonicalModelPrefersExplicitCanonicalModel(t *testing.T) {
	t.Setenv("OPENAI_TAG_MODEL", "gpt-test-tag")
	t.Setenv("OPENAI_CANONICAL_MODEL", "gpt-test-canonical")

	canonicalModel := openAICanonicalModelFromEnv(os.Getenv("OPENAI_TAG_MODEL"))
	if canonicalModel != "gpt-test-canonical" {
		t.Fatalf("expected canonical model override %q, got %q", "gpt-test-canonical", canonicalModel)
	}
}
