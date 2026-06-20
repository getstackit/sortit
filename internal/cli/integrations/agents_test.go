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
	if err := installSkillTargets(baseDir, targets, "1.2.3", &out); err != nil {
		t.Fatalf("installSkillTargets() error = %v", err)
	}

	claudeSkillPath := filepath.Join(baseDir, claudeSkillsDir, "sortit-search", "SKILL.md")
	claudeContent, err := os.ReadFile(claudeSkillPath)
	if err != nil {
		t.Fatalf("read installed claude skill: %v", err)
	}
	if !strings.Contains(string(claudeContent), "version: 1.2.3") {
		t.Fatalf("installed claude skill missing version replacement: %s", string(claudeContent))
	}

	codexSkillPath := filepath.Join(baseDir, codexSkillsDir, "sortit-search", "SKILL.md")
	codexContent, err := os.ReadFile(codexSkillPath)
	if err != nil {
		t.Fatalf("read installed codex skill: %v", err)
	}
	codexText := string(codexContent)
	if strings.Contains(codexText, "\nversion: 1.2.3") {
		t.Fatalf("installed codex skill should keep frontmatter minimal: %s", codexText)
	}
	if !strings.Contains(codexText, "<!-- sortit-version: 1.2.3 -->") {
		t.Fatalf("installed codex skill missing hidden version marker: %s", codexText)
	}
	if !strings.Contains(codexText, `description: "Search Sortit issues from natural-language symptoms, product areas, or customer quotes. Use when you need to find the best existing issues before creating or editing anything."`) {
		t.Fatalf("installed codex skill missing encoded description: %s", codexText)
	}

	if _, err := os.Stat(filepath.Join(baseDir, claudeSkillsDir, "sortit-create", "SKILL.md")); err != nil {
		t.Fatalf("stat claude create skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, codexSkillsDir, "sortit-explore", "SKILL.md")); err != nil {
		t.Fatalf("stat codex explore skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, codexSkillsDir, "sortit-next", "SKILL.md")); err != nil {
		t.Fatalf("stat codex next skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, codexSkillsDir, "sortit-memory", "SKILL.md")); err != nil {
		t.Fatalf("stat codex memory skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, codexSkillsDir, "sortit-wrap-up", "SKILL.md")); err != nil {
		t.Fatalf("stat codex wrap-up skill: %v", err)
	}
	metadata, err := os.ReadFile(filepath.Join(baseDir, codexSkillsDir, "sortit-search", "agents", "openai.yaml"))
	if err != nil {
		t.Fatalf("read codex metadata: %v", err)
	}
	metadataText := string(metadata)
	if !strings.Contains(metadataText, `display_name: "Sortit Search"`) {
		t.Fatalf("codex metadata missing display name: %s", metadataText)
	}
	if !strings.Contains(metadataText, `default_prompt: "Use $sortit-search to search Sortit issues from natural-language symptoms, product areas, or customer quotes. Use when you need to find the best existing issues before creating or editing anything."`) {
		t.Fatalf("codex metadata missing default prompt: %s", metadataText)
	}
	if _, err := os.Stat(filepath.Join(baseDir, claudeSkillsDir, "sortit-search", "agents", "openai.yaml")); !os.IsNotExist(err) {
		t.Fatalf("claude install should not include openai metadata, err=%v", err)
	}
}

func TestInstallSkillTargetsWithInstructions(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	existingCodexPath := filepath.Join(baseDir, codexInstructionsPath)
	if err := os.MkdirAll(filepath.Dir(existingCodexPath), 0o755); err != nil {
		t.Fatalf("mkdir codex instructions dir: %v", err)
	}
	if err := os.WriteFile(existingCodexPath, []byte("# Existing\n\nKeep this.\n"), 0o600); err != nil {
		t.Fatalf("write existing codex instructions: %v", err)
	}

	var out bytes.Buffer
	targets := targetsForFormats([]agentSkillFormat{agentSkillFormatClaude, agentSkillFormatCodex})
	if err := installSkillTargetsWithOptions(baseDir, targets, false, "1.2.3", true, &out); err != nil {
		t.Fatalf("installSkillTargetsWithOptions() error = %v", err)
	}

	claudeContent, err := os.ReadFile(filepath.Join(baseDir, claudeInstructionsPath))
	if err != nil {
		t.Fatalf("read claude instructions: %v", err)
	}
	if !strings.Contains(string(claudeContent), "Sortit Agent Workflow") {
		t.Fatalf("claude instructions missing workflow block: %s", string(claudeContent))
	}

	codexContent, err := os.ReadFile(existingCodexPath)
	if err != nil {
		t.Fatalf("read codex instructions: %v", err)
	}
	codexText := string(codexContent)
	if !strings.Contains(codexText, "Keep this.") {
		t.Fatalf("codex instructions should preserve existing content: %s", codexText)
	}
	if strings.Count(codexText, sortitInstructionsBegin) != 1 {
		t.Fatalf("expected one managed block, got: %s", codexText)
	}

	if err := installSkillTargetsWithOptions(baseDir, targets, true, "1.2.4", true, &out); err != nil {
		t.Fatalf("reinstall with instructions error = %v", err)
	}
	codexContent, err = os.ReadFile(existingCodexPath)
	if err != nil {
		t.Fatalf("read codex instructions after reinstall: %v", err)
	}
	codexText = string(codexContent)
	if strings.Count(codexText, sortitInstructionsBegin) != 1 {
		t.Fatalf("expected managed block replacement, got: %s", codexText)
	}
	if !strings.Contains(codexText, "1.2.4") {
		t.Fatalf("expected updated version in managed block: %s", codexText)
	}
	if !strings.Contains(codexText, "$sortit-* skills") {
		t.Fatalf("expected codex-specific instructions: %s", codexText)
	}
}

func TestInstallSkillTargetsRequiresForceForDifferentVersion(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	var out bytes.Buffer
	targets := targetsForFormats([]agentSkillFormat{agentSkillFormatCodex})

	if err := installSkillTargets(baseDir, targets, "1.0.0", &out); err != nil {
		t.Fatalf("initial installSkillTargets() error = %v", err)
	}

	out.Reset()
	err := installSkillTargets(baseDir, targets, "2.0.0", &out)
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

	staleDir := filepath.Join(baseDir, codexSkillsDir, "sortit-search", "stale")
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

	if err := installSkillTargets(baseDir, targets, "1.2.3", &out); err != nil {
		t.Fatalf("installSkillTargets() error = %v", err)
	}

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed, err=%v", err)
	}
	if _, err := os.Stat(unmanagedPath); err != nil {
		t.Fatalf("expected unmanaged skill to remain, err=%v", err)
	}
}

func TestExtractVersionFromCodexMarker(t *testing.T) {
	t.Parallel()

	content := `---
name: sortit-search
description: "Search issues"
---

# Sortit Search

<!-- sortit-version: 2.0.0 -->
`

	if got := extractVersion(content); got != "2.0.0" {
		t.Fatalf("extractVersion() = %q, want %q", got, "2.0.0")
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
