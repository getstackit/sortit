package issuemap

type TagRelevance struct {
	Tag       string  `json:"tag"`
	Relevance float64 `json:"relevance"`
}

type Issue struct {
	ID   string
	Raw  string
	Tags []TagRelevance
}

type TagDefinition struct {
	Name        string
	Description string
}

func AllTags() []string {
	return issueDataset().tags
}

func AllTagDefinitions() []TagDefinition {
	definitions := make([]TagDefinition, len(tagCatalog))
	for i, spec := range tagCatalog {
		definitions[i] = TagDefinition{
			Name:        spec.Name,
			Description: spec.Description,
		}
	}
	return definitions
}

func AllIssues() []Issue {
	return issueDataset().issues
}
