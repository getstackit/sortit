package integrations

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkillTargets(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	var out bytes.Buffer

	targets := targetsForFormats([]agentSkillFormat{agentSkillFormatClaude, agentSkillFormatCodex})
	if err := installSkillTargets(baseDir, targets, false, "1.2.3", &out); err != nil {
		t.Fatalf("installSkillTargets() error = %v", err)
	}

	claudeSkillPath := filepath.Join(baseDir, claudeSkillsDir, "splat-search", "SKILL.md")
	claudeContent, err := os.ReadFile(claudeSkillPath)
	if err != nil {
		t.Fatalf("read installed claude skill: %v", err)
	}
	if !strings.Contains(string(claudeContent), "version: 1.2.3") {
		t.Fatalf("installed claude skill missing version replacement: %s", string(claudeContent))
	}

	codexSkillPath := filepath.Join(baseDir, codexSkillsDir, "splat-search", "SKILL.md")
	codexContent, err := os.ReadFile(codexSkillPath)
	if err != nil {
		t.Fatalf("read installed codex skill: %v", err)
	}
	if !strings.Contains(string(codexContent), "version: 1.2.3") {
		t.Fatalf("installed codex skill missing version replacement: %s", string(codexContent))
	}

	if _, err := os.Stat(filepath.Join(baseDir, claudeSkillsDir, "splat-create", "SKILL.md")); err != nil {
		t.Fatalf("stat claude create skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, codexSkillsDir, "splat-explore", "SKILL.md")); err != nil {
		t.Fatalf("stat codex explore skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, codexSkillsDir, "splat-search", "agents", "openai.yaml")); err != nil {
		t.Fatalf("stat codex openai metadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, claudeSkillsDir, "splat-search", "agents", "openai.yaml")); !os.IsNotExist(err) {
		t.Fatalf("claude install should not include openai metadata, err=%v", err)
	}
}

func TestInstallSkillTargetsRequiresForceForDifferentVersion(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	var out bytes.Buffer
	targets := targetsForFormats([]agentSkillFormat{agentSkillFormatCodex})

	if err := installSkillTargets(baseDir, targets, false, "1.0.0", &out); err != nil {
		t.Fatalf("initial installSkillTargets() error = %v", err)
	}

	out.Reset()
	err := installSkillTargets(baseDir, targets, false, "2.0.0", &out)
	if err == nil {
		t.Fatal("expected error for existing installation without force")
	}
	if !strings.Contains(out.String(), "Run with --force to overwrite.") {
		t.Fatalf("expected overwrite guidance, got %q", out.String())
	}
}

func TestSelectInstallTargetsWithFormats(t *testing.T) {
	t.Parallel()

	targets, err := selectInstallTargets(t.TempDir(), []string{"claude,codex"}, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("selectInstallTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].format != agentSkillFormatClaude || targets[1].format != agentSkillFormatCodex {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}

func TestSelectInstallTargetsFallsBackToExistingDirsWhenNonInteractive(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(baseDir, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}

	targets, err := selectInstallTargets(baseDir, nil, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("selectInstallTargets() error = %v", err)
	}
	if len(targets) != 1 || targets[0].format != agentSkillFormatCodex {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}

func TestSelectInstallTargetsErrorsWithoutTTYOrExistingDirs(t *testing.T) {
	t.Parallel()

	_, err := selectInstallTargets(t.TempDir(), nil, strings.NewReader(""), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected selection error")
	}
	if !strings.Contains(err.Error(), "use --format=claude or --format=codex") {
		t.Fatalf("unexpected error: %v", err)
	}
}
