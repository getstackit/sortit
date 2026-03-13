package integrations

import "embed"

//go:embed agents/templates/skills/*/SKILL.md agents/templates/common/*.yaml
var agentTemplates embed.FS
