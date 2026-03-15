package domain

import "strings"

// TagRelevance represents the relevance score of a tag to an issue.
type TagRelevance struct {
	Tag       string  `json:"tag"`
	Relevance float64 `json:"relevance"`
}

// NormalizeTagName lowercases and collapses whitespace in a tag name.
func NormalizeTagName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}
