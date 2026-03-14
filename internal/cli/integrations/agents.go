package integrations

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const (
	claudeSkillsDir   = ".claude/skills"
	codexSkillsDir    = ".codex/skills"
	claudeSkillsLabel = "~/.claude/skills"
	codexSkillsLabel  = "~/.codex/skills"
)

type agentSkillFormat string

const (
	agentSkillFormatClaude agentSkillFormat = "claude"
	agentSkillFormatCodex  agentSkillFormat = "codex"
)

type fileGroup struct {
	templatePath string
	destPath     string
	replaceVer   bool
}

type agentInstallTarget struct {
	format      agentSkillFormat
	skillsDir   string
	displayPath string
}

func NewAgentsCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "agent",
		Short:        "Manage Claude Code and Codex skill files for Splat",
		SilenceUsage: true,
	}

	cmd.AddCommand(newAgentInstallCmd(version))

	return cmd
}

func newAgentInstallCmd(version string) *cobra.Command {
	var force bool
	var formats []string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Splat skill files",
		Long: `Install Splat skill files into your home directory.

This creates one or both skill sets under:
  - ~/.claude/skills
  - ~/.codex/skills

The installed skill teaches agents to use the Splat CLI, starting with issue
search, create, and explore workflows plus the rest of the issue operations.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("resolve home directory: %w", err)
			}
			targets, err := selectInstallTargets(baseDir, formats, cmd.InOrStdin(), cmd.OutOrStdout())
			if err != nil {
				return err
			}
			return installSkillTargets(baseDir, targets, force, version, cmd.OutOrStdout())
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing installation")
	cmd.Flags().StringSliceVar(&formats, "format", nil, "Skill format(s) to install (claude,codex). Repeat flag or use comma-separated values")

	return cmd
}

func installSkillTargets(baseDir string, targets []agentInstallTarget, force bool, version string, out io.Writer) error {
	for _, target := range targets {
		if err := checkExistingInstallation(baseDir, target, force, version, out); err != nil {
			return err
		}
	}

	for _, target := range targets {
		clearedDirs := make(map[string]struct{})
		for _, group := range buildSkillFileGroups(target, skillDefinitions) {
			destDir := filepath.Join(baseDir, filepath.Dir(group.destPath))
			if _, ok := clearedDirs[destDir]; !ok {
				if err := os.RemoveAll(destDir); err != nil {
					return fmt.Errorf("clear directory %s: %w", destDir, err)
				}
				clearedDirs[destDir] = struct{}{}
			}
			if err := installFileGroup(baseDir, group, version); err != nil {
				return err
			}
		}
	}

	_, _ = fmt.Fprintf(out, "Installed %d Splat skills:\n", len(skillDefinitions))
	for _, target := range targets {
		_, _ = fmt.Fprintf(out, "  %s (%d skills)\n", target.displayPath, len(skillDefinitions))
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Installed skills:")
	for _, skill := range skillDefinitions {
		_, _ = fmt.Fprintf(out, "  %s\n", skill.name)
	}

	return nil
}

func buildSkillFileGroups(target agentInstallTarget, skills []skillDefinition) []fileGroup {
	groups := make([]fileGroup, 0, len(skills))
	for _, skill := range skills {
		groups = append(groups, fileGroup{
			templatePath: skill.templatePath,
			destPath:     filepath.Join(target.skillsDir, skill.name, "SKILL.md"),
			replaceVer:   true,
		})
	}
	return groups
}

func installFileGroup(baseDir string, group fileGroup, version string) error {
	destPath := filepath.Join(baseDir, group.destPath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", filepath.Dir(destPath), err)
	}

	content, err := agentTemplates.ReadFile(group.templatePath)
	if err != nil {
		return fmt.Errorf("read template %s: %w", group.templatePath, err)
	}
	if group.replaceVer {
		content = []byte(strings.ReplaceAll(string(content), "{{VERSION}}", version))
	}
	if err := os.WriteFile(destPath, content, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", destPath, err)
	}

	return nil
}

func checkExistingInstallation(baseDir string, target agentInstallTarget, force bool, version string, out io.Writer) error {
	for _, skill := range skillDefinitions {
		skillPath := filepath.Join(baseDir, target.skillsDir, skill.name, "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read existing installation: %w", err)
		}

		if force {
			continue
		}

		existingVersion := extractVersion(string(content))
		if version != "" && existingVersion == version {
			continue
		}

		_, _ = fmt.Fprintf(out, "Found existing Splat skill at %s/%s", target.displayPath, skill.name)
		if existingVersion != "" {
			_, _ = fmt.Fprintf(out, " (version %s)", existingVersion)
		}
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Run with --force to overwrite.")
		return fmt.Errorf("existing installation found")
	}
	return nil
}

func selectInstallTargets(baseDir string, rawFormats []string, in io.Reader, out io.Writer) ([]agentInstallTarget, error) {
	if len(rawFormats) > 0 {
		formats, err := parseAgentSkillFormats(rawFormats)
		if err != nil {
			return nil, err
		}
		return targetsForFormats(formats), nil
	}

	hasClaudeDir := dirExists(filepath.Join(baseDir, ".claude"))
	hasCodexDir := dirExists(filepath.Join(baseDir, ".codex"))
	preSelected := []bool{hasClaudeDir, hasCodexDir}
	if !hasClaudeDir && !hasCodexDir {
		preSelected = []bool{true, false}
	}

	selected, err := promptMultiSelectWithDefaults(
		in,
		out,
		"Which skill format(s) would you like to install?",
		[]string{
			"Claude Code - Claude Code CLI skill format (~/.claude/skills/splat-*)",
			"Codex - Codex skill format (~/.codex/skills/splat-*)",
		},
		preSelected,
	)
	if errors.Is(err, errInteractiveDisabled) {
		fallback := make([]agentSkillFormat, 0, 2)
		if hasClaudeDir {
			fallback = append(fallback, agentSkillFormatClaude)
		}
		if hasCodexDir {
			fallback = append(fallback, agentSkillFormatCodex)
		}
		if len(fallback) > 0 {
			return targetsForFormats(fallback), nil
		}
		return nil, fmt.Errorf("format selection requires interactive mode; use --format=claude or --format=codex")
	}
	if err != nil {
		return nil, err
	}

	formats, err := parseSelectedFormatLabels(selected)
	if err != nil {
		return nil, err
	}
	if len(formats) == 0 {
		return nil, fmt.Errorf("at least one format must be selected")
	}
	return targetsForFormats(formats), nil
}

func parseSelectedFormatLabels(selected []string) ([]agentSkillFormat, error) {
	formats := make([]agentSkillFormat, 0, len(selected))
	for _, label := range selected {
		switch {
		case strings.HasPrefix(label, "Claude Code -"):
			formats = append(formats, agentSkillFormatClaude)
		case strings.HasPrefix(label, "Codex -"):
			formats = append(formats, agentSkillFormatCodex)
		default:
			return nil, fmt.Errorf("unsupported selected format label %q", label)
		}
	}
	return dedupeFormats(formats), nil
}

func parseAgentSkillFormats(rawValues []string) ([]agentSkillFormat, error) {
	formats := make([]agentSkillFormat, 0, len(rawValues))
	for _, raw := range rawValues {
		for _, part := range strings.Split(raw, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			format, err := parseAgentSkillFormat(trimmed)
			if err != nil {
				return nil, err
			}
			formats = append(formats, format)
		}
	}
	if len(formats) == 0 {
		return nil, fmt.Errorf("at least one format must be provided")
	}
	return dedupeFormats(formats), nil
}

func parseAgentSkillFormat(raw string) (agentSkillFormat, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case string(agentSkillFormatClaude), "claude-code":
		return agentSkillFormatClaude, nil
	case string(agentSkillFormatCodex):
		return agentSkillFormatCodex, nil
	default:
		return "", fmt.Errorf("unsupported format %q (expected claude or codex)", raw)
	}
}

func targetsForFormats(formats []agentSkillFormat) []agentInstallTarget {
	targets := make([]agentInstallTarget, 0, len(formats))
	for _, format := range formats {
		switch format {
		case agentSkillFormatClaude:
			targets = append(targets, agentInstallTarget{
				format:      agentSkillFormatClaude,
				skillsDir:   claudeSkillsDir,
				displayPath: claudeSkillsLabel,
			})
		case agentSkillFormatCodex:
			targets = append(targets, agentInstallTarget{
				format:      agentSkillFormatCodex,
				skillsDir:   codexSkillsDir,
				displayPath: codexSkillsLabel,
			})
		}
	}
	return targets
}

func dedupeFormats(formats []agentSkillFormat) []agentSkillFormat {
	seen := make(map[agentSkillFormat]bool, len(formats))
	out := make([]agentSkillFormat, 0, len(formats))
	for _, format := range formats {
		if seen[format] {
			continue
		}
		seen[format] = true
		out = append(out, format)
	}
	return out
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

var errInteractiveDisabled = errors.New("interactive prompts are disabled")

func promptMultiSelectWithDefaults(in io.Reader, out io.Writer, title string, options []string, preSelected []bool) ([]string, error) {
	if !isInteractive(in, out) {
		return nil, errInteractiveDisabled
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no options provided")
	}

	_, _ = fmt.Fprintln(out, title)
	defaultLabels := make([]string, 0, len(options))
	for i, option := range options {
		marker := " "
		if i < len(preSelected) && preSelected[i] {
			marker = "x"
			defaultLabels = append(defaultLabels, strconv.Itoa(i+1))
		}
		_, _ = fmt.Fprintf(out, "  [%s] %d. %s\n", marker, i+1, option)
	}

	defaultValue := strings.Join(defaultLabels, ",")
	if defaultValue == "" {
		defaultValue = "1"
	}
	_, _ = fmt.Fprintf(out, "Select one or more options (comma-separated) [%s]: ", defaultValue)

	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = defaultValue
	}

	parts := strings.Split(line, ",")
	selected := make([]string, 0, len(parts))
	seen := make(map[int]bool, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		index, err := strconv.Atoi(part)
		if err != nil || index < 1 || index > len(options) {
			return nil, fmt.Errorf("invalid selection %q", part)
		}
		if seen[index] {
			continue
		}
		seen[index] = true
		selected = append(selected, options[index-1])
	}
	return selected, nil
}

func isInteractive(in io.Reader, out io.Writer) bool {
	inFile, ok := in.(*os.File)
	if !ok {
		return false
	}
	outFile, ok := out.(*os.File)
	if !ok {
		return false
	}
	inInfo, err := inFile.Stat()
	if err != nil || (inInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	outInfo, err := outFile.Stat()
	if err != nil || (outInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	return true
}

func extractVersion(content string) string {
	lines := strings.Split(content, "\n")
	inFrontmatter := false

	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break
		}

		if inFrontmatter && strings.HasPrefix(line, "version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "version:"))
		}
	}

	return ""
}
