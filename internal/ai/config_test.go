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
