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

	claudeInstructionsPath = ".claude/CLAUDE.md"
	codexInstructionsPath  = ".codex/AGENTS.md"

	sortitInstructionsBegin = "<!-- sortit-agent-instructions:start -->"
	sortitInstructionsEnd   = "<!-- sortit-agent-instructions:end -->"
)

type agentSkillFormat string

const (
	agentSkillFormatClaude agentSkillFormat = "claude"
	agentSkillFormatCodex  agentSkillFormat = "codex"
)

type fileGroup struct {
	templatePath string
	destPath     string
	skillName    string
	summary      string
	format       agentSkillFormat
}

type agentInstallTarget struct {
	format      agentSkillFormat
	skillsDir   string
	displayPath string
}

func NewAgentsCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "agent",
		Short:        "Manage Claude Code and Codex skill files for Sortit",
		SilenceUsage: true,
	}

	cmd.AddCommand(newAgentInstallCmd(version))

	return cmd
}

func newAgentInstallCmd(version string) *cobra.Command {
	var force bool
	var formats []string
	var instructions bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Sortit skill files",
		Long: `Install Sortit skill files into your home directory.

This creates one or both skill sets under:
  - ~/.claude/skills
  - ~/.codex/skills

The installed skill teaches agents to use the Sortit CLI, starting with issue
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
			return installSkillTargetsWithOptions(baseDir, targets, force, version, instructions, cmd.OutOrStdout())
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing installation")
	cmd.Flags().StringSliceVar(&formats, "format", nil, "Skill format(s) to install (claude,codex). Repeat flag or use comma-separated values")
	cmd.Flags().BoolVar(&instructions, "instructions", false, "Also install managed always-on Sortit workflow instructions")

	return cmd
}

func installSkillTargets(baseDir string, targets []agentInstallTarget, version string, out io.Writer) error {
	return installSkillTargetsWithOptions(baseDir, targets, false, version, false, out)
}

func installSkillTargetsWithOptions(baseDir string, targets []agentInstallTarget, force bool, version string, instructions bool, out io.Writer) error {
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
		if instructions {
			if err := installInstructionFile(baseDir, target, version); err != nil {
				return err
			}
		}
	}

	_, _ = fmt.Fprintf(out, "Installed %d Sortit skills:\n", len(skillDefinitions))
	for _, target := range targets {
		_, _ = fmt.Fprintf(out, "  %s (%d skills)\n", target.displayPath, len(skillDefinitions))
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Installed skills:")
	for _, skill := range skillDefinitions {
		_, _ = fmt.Fprintf(out, "  %s\n", skill.name)
	}
	if instructions {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Installed always-on Sortit workflow instructions.")
	}

	return nil
}

func installInstructionFile(baseDir string, target agentInstallTarget, version string) error {
	path, err := instructionPathForTarget(target)
	if err != nil {
		return err
	}
	destPath := filepath.Join(baseDir, path)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return fmt.Errorf("create directory %s: %w", filepath.Dir(destPath), err)
	}

	existing, err := os.ReadFile(destPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", destPath, err)
	}

	content := upsertManagedInstructionBlock(string(existing), sortitInstructionBlock(target.format, version))
	if err := os.WriteFile(destPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", destPath, err)
	}
	return nil
}

func instructionPathForTarget(target agentInstallTarget) (string, error) {
	switch target.format {
	case agentSkillFormatClaude:
		return claudeInstructionsPath, nil
	case agentSkillFormatCodex:
		return codexInstructionsPath, nil
	default:
		return "", fmt.Errorf("unknown agent format %q", target.format)
	}
}

func sortitInstructionBlock(format agentSkillFormat, version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "dev"
	}

	agentNote := "Use the installed Sortit skills by name when a workflow matches."
	switch format {
	case agentSkillFormatClaude:
		agentNote = "Claude Code should use the installed Sortit skills and prefer MCP tools when they are available."
	case agentSkillFormatCodex:
		agentNote = "Codex should use the installed $sortit-* skills and prefer MCP tools when they are available."
	}

	return sortitInstructionsBegin + "\n" +
		"## Sortit Agent Workflow\n\n" +
		"Use Sortit as the durable work loop during coding, review, planning, and debugging.\n\n" +
		"- " + agentNote + "\n" +
		"- Search Sortit before creating follow-on work.\n" +
		"- Recall relevant memories before deciding — prior decisions, constraints, and patterns should guide the work, not be rediscovered or contradicted.\n" +
		"- Record progress on the relevant issue when meaningful work is done.\n" +
		"- Create or refine follow-on issues instead of leaving deferred work only in chat.\n" +
		"- Create a memory for durable decisions, lessons, constraints, patterns, or references.\n" +
		"- Use the Sortit wrap-up checklist before final responses for non-trivial work.\n" +
		"- Leave uncertain corpus-wide cleanup as proposals for human review.\n\n" +
		"Installed by `sortit agent install --instructions` (" + version + ").\n" +
		sortitInstructionsEnd + "\n"
}

func upsertManagedInstructionBlock(existing, block string) string {
	existing = strings.TrimRight(existing, "\n")
	start := strings.Index(existing, sortitInstructionsBegin)
	end := strings.Index(existing, sortitInstructionsEnd)
	if start >= 0 && end >= start {
		end += len(sortitInstructionsEnd)
		replacement := strings.TrimRight(block, "\n")
		out := existing[:start] + replacement + existing[end:]
		return strings.TrimRight(out, "\n") + "\n"
	}
	if existing == "" {
		return block
	}
	return existing + "\n\n" + block
}

func buildSkillFileGroups(target agentInstallTarget, skills []skillDefinition) []fileGroup {
	groups := make([]fileGroup, 0, len(skills))
	for _, skill := range skills {
		groups = append(groups, fileGroup{
			templatePath: skill.templatePath(target.format),
			destPath:     filepath.Join(target.skillsDir, skill.name, "SKILL.md"),
			skillName:    skill.name,
			summary:      skill.summary,
			format:       target.format,
		})
	}
	return groups
}

func installFileGroup(baseDir string, group fileGroup, version string) error {
	destPath := filepath.Join(baseDir, group.destPath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return fmt.Errorf("create directory %s: %w", filepath.Dir(destPath), err)
	}

	content, err := agentTemplates.ReadFile(group.templatePath)
	if err != nil {
		return fmt.Errorf("read template %s: %w", group.templatePath, err)
	}
	content, err = renderSkillContent(group.format, group.skillName, content, version)
	if err != nil {
		return fmt.Errorf("render %s: %w", group.skillName, err)
	}
	if err := os.WriteFile(destPath, content, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", destPath, err)
	}
	if group.format == agentSkillFormatCodex {
		metadata, err := renderCodexSkillMetadata(group.skillName, group.summary)
		if err != nil {
			return fmt.Errorf("render metadata for %s: %w", group.skillName, err)
		}
		agentsDir := filepath.Join(filepath.Dir(destPath), "agents")
		if err := os.MkdirAll(agentsDir, 0o750); err != nil {
			return fmt.Errorf("create directory %s: %w", agentsDir, err)
		}
		if err := os.WriteFile(filepath.Join(agentsDir, "openai.yaml"), metadata, 0o600); err != nil {
			return fmt.Errorf("write metadata for %s: %w", group.skillName, err)
		}
	}

	return nil
}

func renderSkillContent(format agentSkillFormat, name string, content []byte, version string) ([]byte, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "dev"
	}

	switch format {
	case agentSkillFormatClaude:
		return []byte(strings.ReplaceAll(string(content), "{{VERSION}}", version)), nil
	case agentSkillFormatCodex:
		return renderCodexSkillContent(name, content, version)
	default:
		return nil, fmt.Errorf("unknown agent format %q", format)
	}
}

func renderCodexSkillContent(name string, content []byte, version string) ([]byte, error) {
	frontmatter, body, ok := splitFrontmatter(content)
	if !ok {
		return nil, fmt.Errorf("missing frontmatter")
	}
	description := frontmatterValue(frontmatter, "description")
	if description == "" {
		return nil, fmt.Errorf("missing description")
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(name)
	b.WriteString("\n")
	b.WriteString("description: ")
	b.WriteString(encodeYAMLScalar(description))
	b.WriteString("\n---\n")
	b.Write(body)
	if len(body) == 0 || body[len(body)-1] != '\n' {
		b.WriteString("\n")
	}
	b.WriteString("\n<!-- sortit-version: ")
	b.WriteString(version)
	b.WriteString(" -->\n")
	return []byte(b.String()), nil
}

// renderCodexSkillMetadata builds the Codex interface metadata (openai.yaml)
// from the skill's short summary. The summary is kept separate from the
// SKILL.md description so the description can be a longer, trigger-oriented
// activation signal without bloating the display metadata.
func renderCodexSkillMetadata(name, summary string) ([]byte, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil, fmt.Errorf("missing summary for %s", name)
	}

	var b strings.Builder
	b.WriteString("interface:\n")
	b.WriteString("  display_name: ")
	b.WriteString(encodeYAMLScalar(displayNameForSkill(name)))
	b.WriteString("\n")
	b.WriteString("  short_description: ")
	b.WriteString(encodeYAMLScalar(summary))
	b.WriteString("\n")
	b.WriteString("  default_prompt: ")
	b.WriteString(encodeYAMLScalar(defaultPromptForSkill(name, summary)))
	b.WriteString("\n")
	return []byte(b.String()), nil
}

func splitFrontmatter(content []byte) (frontmatter, body []byte, ok bool) {
	const marker = "---\n"
	if !strings.HasPrefix(string(content), marker) {
		return nil, nil, false
	}
	rest := content[len(marker):]
	idx := strings.Index(string(rest), marker)
	if idx < 0 {
		return nil, nil, false
	}
	return rest[:idx], rest[idx+len(marker):], true
}

func frontmatterValue(frontmatter []byte, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(string(frontmatter), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		return strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), `"'`)
	}
	return ""
}

func encodeYAMLScalar(raw string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range raw {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func displayNameForSkill(name string) string {
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func lowercaseFirst(raw string) string {
	if raw == "" {
		return raw
	}
	return strings.ToLower(raw[:1]) + raw[1:]
}

func defaultPromptForSkill(name, description string) string {
	prompt := fmt.Sprintf("Use $%s to %s", name, lowercaseFirst(description))
	if strings.HasSuffix(prompt, ".") || strings.HasSuffix(prompt, "!") || strings.HasSuffix(prompt, "?") {
		return prompt
	}
	return prompt + "."
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

		_, _ = fmt.Fprintf(out, "Found existing Sortit skill at %s/%s", target.displayPath, skill.name)
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
			"Claude Code - Claude Code CLI skill format (~/.claude/skills/sortit-*)",
			"Codex - Codex skill format (~/.codex/skills/sortit-*)",
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
	bodyStart := 0

	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			bodyStart = i + 1
			break
		}

		if inFrontmatter && strings.HasPrefix(line, "version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "version:"))
		}
	}

	const prefix = "<!-- sortit-version:"
	const suffix = "-->"
	for _, line := range lines[bodyStart:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, suffix) {
			continue
		}
		value := strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), suffix)
		return strings.TrimSpace(value)
	}

	return ""
}
