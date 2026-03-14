package integrations

import "embed"

//go:embed agents/templates/skills/*/SKILL.md
var agentTemplates embed.FS
