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

	targets := targetsForFormats([]agentSkillFormat{agentSkillFormatClaude, agentSkillFormatCodex}, installScope{name: "global", global: true})
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
	if !strings.Contains(codexText, "name: sortit-search") {
		t.Fatalf("installed codex skill missing name in frontmatter: %s", codexText)
	}
	if !strings.Contains(codexText, `description: "Search Sortit issues`) {
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
	if !strings.Contains(metadataText, `short_description: "Search Sortit issues by symptom, area, or quote"`) {
		t.Fatalf("codex metadata missing short description: %s", metadataText)
	}
	if !strings.Contains(metadataText, `default_prompt: "Use $sortit-search to search Sortit issues by symptom, area, or quote."`) {
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
	targets := targetsForFormats([]agentSkillFormat{agentSkillFormatClaude, agentSkillFormatCodex}, installScope{name: "global", global: true})
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
	targets := targetsForFormats([]agentSkillFormat{agentSkillFormatCodex}, installScope{name: "global", global: true})

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
	targets := targetsForFormats([]agentSkillFormat{agentSkillFormatCodex}, installScope{name: "global", global: true})

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

func TestSelectInstallFormatsWithFormats(t *testing.T) {
	t.Parallel()

	scopes := []installScope{{name: "global", baseDir: t.TempDir(), global: true}}
	formats, err := selectInstallFormats(scopes, []string{"claude,codex"}, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("selectInstallFormats() error = %v", err)
	}
	if len(formats) != 2 {
		t.Fatalf("expected 2 formats, got %d", len(formats))
	}
	if formats[0] != agentSkillFormatClaude || formats[1] != agentSkillFormatCodex {
		t.Fatalf("unexpected formats: %+v", formats)
	}
}

func TestSelectInstallFormatsFallsBackToExistingDirsWhenNonInteractive(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(baseDir, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}

	scopes := []installScope{{name: "local", baseDir: baseDir}}
	formats, err := selectInstallFormats(scopes, nil, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("selectInstallFormats() error = %v", err)
	}
	if len(formats) != 1 || formats[0] != agentSkillFormatCodex {
		t.Fatalf("unexpected formats: %+v", formats)
	}
}

func TestSelectInstallFormatsDetectsDirsAcrossScopes(t *testing.T) {
	t.Parallel()

	localDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(localDir, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	globalDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(globalDir, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}

	// A .claude in the local scope and a .codex in the global scope should both
	// be detected so the non-interactive fallback installs both formats.
	scopes := []installScope{
		{name: "global", baseDir: globalDir, global: true},
		{name: "local", baseDir: localDir},
	}
	formats, err := selectInstallFormats(scopes, nil, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("selectInstallFormats() error = %v", err)
	}
	if len(formats) != 2 {
		t.Fatalf("expected 2 formats from cross-scope detection, got %+v", formats)
	}
}

func TestSelectInstallFormatsErrorsWithoutTTYOrExistingDirs(t *testing.T) {
	t.Parallel()

	scopes := []installScope{{name: "global", baseDir: t.TempDir(), global: true}}
	_, err := selectInstallFormats(scopes, nil, strings.NewReader(""), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected selection error")
	}
	if !strings.Contains(err.Error(), "use --format=claude or --format=codex") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveInstallScopes(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	tests := []struct {
		name        string
		local       bool
		global      bool
		wantNames   []string
		wantBaseDir map[string]string
	}{
		{
			name:        "default is global",
			wantNames:   []string{"global"},
			wantBaseDir: map[string]string{"global": home},
		},
		{
			name:        "global only",
			global:      true,
			wantNames:   []string{"global"},
			wantBaseDir: map[string]string{"global": home},
		},
		{
			name:        "local only",
			local:       true,
			wantNames:   []string{"local"},
			wantBaseDir: map[string]string{"local": cwd},
		},
		{
			name:        "both, global first",
			local:       true,
			global:      true,
			wantNames:   []string{"global", "local"},
			wantBaseDir: map[string]string{"global": home, "local": cwd},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scopes, err := resolveInstallScopes(tc.local, tc.global)
			if err != nil {
				t.Fatalf("resolveInstallScopes() error = %v", err)
			}
			if len(scopes) != len(tc.wantNames) {
				t.Fatalf("expected %d scopes, got %+v", len(tc.wantNames), scopes)
			}
			for i, want := range tc.wantNames {
				if scopes[i].name != want {
					t.Fatalf("scope[%d].name = %q, want %q", i, scopes[i].name, want)
				}
				if scopes[i].baseDir != tc.wantBaseDir[want] {
					t.Fatalf("scope %q baseDir = %q, want %q", want, scopes[i].baseDir, tc.wantBaseDir[want])
				}
			}
			// global scope drives the ~/ display prefix; local does not.
			for _, s := range scopes {
				if s.name == "global" && !s.global {
					t.Fatalf("global scope should set global=true")
				}
				if s.name == "local" && s.global {
					t.Fatalf("local scope should set global=false")
				}
			}
		})
	}
}

func TestTargetsForFormatsScopeLabels(t *testing.T) {
	t.Parallel()

	formats := []agentSkillFormat{agentSkillFormatClaude, agentSkillFormatCodex}

	global := targetsForFormats(formats, installScope{name: "global", baseDir: "/home/x", global: true})
	if global[0].displayPath != "~/.claude/skills" || global[1].displayPath != "~/.codex/skills" {
		t.Fatalf("unexpected global labels: %q, %q", global[0].displayPath, global[1].displayPath)
	}

	local := targetsForFormats(formats, installScope{name: "local", baseDir: "/repo"})
	if local[0].displayPath != ".claude/skills" || local[1].displayPath != ".codex/skills" {
		t.Fatalf("unexpected local labels: %q, %q", local[0].displayPath, local[1].displayPath)
	}
	// skillsDir is scope-independent (joined onto baseDir at install time).
	if local[0].skillsDir != claudeSkillsDir || local[1].skillsDir != codexSkillsDir {
		t.Fatalf("unexpected skillsDir: %q, %q", local[0].skillsDir, local[1].skillsDir)
	}
}
