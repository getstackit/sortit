package integrations

type skillDefinition struct {
	name         string
	templatePath string
}

var skillDefinitions = []skillDefinition{
	{name: "sortit-search", templatePath: "agents/templates/skills/sortit-search/SKILL.md"},
	{name: "sortit-create", templatePath: "agents/templates/skills/sortit-create/SKILL.md"},
	{name: "sortit-explore", templatePath: "agents/templates/skills/sortit-explore/SKILL.md"},
	{name: "sortit-get", templatePath: "agents/templates/skills/sortit-get/SKILL.md"},
	{name: "sortit-refine", templatePath: "agents/templates/skills/sortit-refine/SKILL.md"},
	{name: "sortit-progress", templatePath: "agents/templates/skills/sortit-progress/SKILL.md"},
	{name: "sortit-close", templatePath: "agents/templates/skills/sortit-close/SKILL.md"},
	{name: "sortit-assign", templatePath: "agents/templates/skills/sortit-assign/SKILL.md"},
	{name: "sortit-split", templatePath: "agents/templates/skills/sortit-split/SKILL.md"},
	{name: "sortit-combine", templatePath: "agents/templates/skills/sortit-combine/SKILL.md"},
	{name: "sortit-link", templatePath: "agents/templates/skills/sortit-link/SKILL.md"},
	{name: "sortit-tags", templatePath: "agents/templates/skills/sortit-tags/SKILL.md"},
	{name: "sortit-people", templatePath: "agents/templates/skills/sortit-people/SKILL.md"},
	{name: "sortit-next", templatePath: "agents/templates/skills/sortit-next/SKILL.md"},
	{name: "sortit-auth", templatePath: "agents/templates/skills/sortit-auth/SKILL.md"},
	{name: "sortit-diagnose", templatePath: "agents/templates/skills/sortit-diagnose/SKILL.md"},
}
