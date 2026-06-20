package integrations

import "embed"

//go:embed agents/templates/claude/skills/*/SKILL.md agents/templates/codex/skills/*/SKILL.md
var agentTemplates embed.FS
