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
	if _, err := os.Stat(filepath.Join(baseDir, codexSkillsDir, "splat-next", "SKILL.md")); err != nil {
		t.Fatalf("stat codex next skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, codexSkillsDir, "splat-search", "agents", "openai.yaml")); !os.IsNotExist(err) {
		t.Fatalf("codex install should not include openai metadata, err=%v", err)
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

func TestInstallSkillTargetsClearsManagedSkillDirectories(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	var out bytes.Buffer
	targets := targetsForFormats([]agentSkillFormat{agentSkillFormatCodex})

	staleDir := filepath.Join(baseDir, codexSkillsDir, "splat-search", "agents")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir stale dir: %v", err)
	}
	stalePath := filepath.Join(staleDir, "openai.yaml")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	unmanagedPath := filepath.Join(baseDir, codexSkillsDir, "custom-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(unmanagedPath), 0o755); err != nil {
		t.Fatalf("mkdir unmanaged dir: %v", err)
	}
	if err := os.WriteFile(unmanagedPath, []byte("custom"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	if err := installSkillTargets(baseDir, targets, false, "1.2.3", &out); err != nil {
		t.Fatalf("installSkillTargets() error = %v", err)
	}

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed, err=%v", err)
	}
	if _, err := os.Stat(unmanagedPath); err != nil {
		t.Fatalf("expected unmanaged skill to remain, err=%v", err)
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
