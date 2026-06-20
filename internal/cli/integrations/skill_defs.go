package integrations

type skillDefinition struct {
	name string
	// summary is a short, human-facing description used for Codex interface
	// metadata (openai.yaml). The SKILL.md description can be longer and more
	// trigger-oriented; this stays concise for display.
	summary string
}

// templatePath returns the embedded template path for the skill in the given
// agent format. Claude and Codex each have an independently authored template
// tree so the content can use format-specific features.
func (s skillDefinition) templatePath(format agentSkillFormat) string {
	switch format {
	case agentSkillFormatCodex:
		return "agents/templates/codex/skills/" + s.name + "/SKILL.md"
	default:
		return "agents/templates/claude/skills/" + s.name + "/SKILL.md"
	}
}

var skillDefinitions = []skillDefinition{
	{name: "sortit-search", summary: "Search Sortit issues by symptom, area, or quote"},
	{name: "sortit-create", summary: "Create a new Sortit issue from raw text"},
	{name: "sortit-explore", summary: "Explore issues related to a known issue"},
	{name: "sortit-get", summary: "Fetch a Sortit issue by ID"},
	{name: "sortit-refine", summary: "Append refinement context to issues"},
	{name: "sortit-progress", summary: "Log progress on issues"},
	{name: "sortit-memory", summary: "Create or review durable Sortit memories"},
	{name: "sortit-recall", summary: "Recall durable memories relevant to the task"},
	{name: "sortit-wrap-up", summary: "Run the end-of-turn Sortit checklist"},
	{name: "sortit-close", summary: "Close one or more issues"},
	{name: "sortit-assign", summary: "Assign or unassign issues"},
	{name: "sortit-split", summary: "Split an issue into child issues"},
	{name: "sortit-combine", summary: "Combine duplicate issues into one"},
	{name: "sortit-link", summary: "Link two issues with a relationship"},
	{name: "sortit-tags", summary: "Inspect the tag catalog"},
	{name: "sortit-people", summary: "Inspect people analytics"},
	{name: "sortit-next", summary: "Find the user's next issue to work on"},
	{name: "sortit-auth", summary: "Authenticate the Sortit CLI"},
	{name: "sortit-diagnose", summary: "Diagnose factor-model and tag quality"},
	{name: "sortit-curate", summary: "Run a propose-only curation sweep"},
}
