package integrations

type skillDefinition struct {
	name         string
	templatePath string
}

var skillDefinitions = []skillDefinition{
	{name: "splat-search", templatePath: "agents/templates/skills/splat-search/SKILL.md"},
	{name: "splat-create", templatePath: "agents/templates/skills/splat-create/SKILL.md"},
	{name: "splat-explore", templatePath: "agents/templates/skills/splat-explore/SKILL.md"},
	{name: "splat-get", templatePath: "agents/templates/skills/splat-get/SKILL.md"},
	{name: "splat-refine", templatePath: "agents/templates/skills/splat-refine/SKILL.md"},
	{name: "splat-progress", templatePath: "agents/templates/skills/splat-progress/SKILL.md"},
	{name: "splat-close", templatePath: "agents/templates/skills/splat-close/SKILL.md"},
	{name: "splat-assign", templatePath: "agents/templates/skills/splat-assign/SKILL.md"},
	{name: "splat-split", templatePath: "agents/templates/skills/splat-split/SKILL.md"},
	{name: "splat-combine", templatePath: "agents/templates/skills/splat-combine/SKILL.md"},
	{name: "splat-link", templatePath: "agents/templates/skills/splat-link/SKILL.md"},
	{name: "splat-tags", templatePath: "agents/templates/skills/splat-tags/SKILL.md"},
	{name: "splat-people", templatePath: "agents/templates/skills/splat-people/SKILL.md"},
	{name: "splat-auth", templatePath: "agents/templates/skills/splat-auth/SKILL.md"},
}
