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

// genericBucketTags lists broad category tags that should be deprioritized
// in favor of more specific subsystem or feature-area tags.
var genericBucketTags = map[string]bool{
	"api":       true,
	"backend":   true,
	"client":    true,
	"front end": true,
	"frontend":  true,
	"server":    true,
	"ui":        true,
	"ux":        true,
}

// IsGenericBucketTag returns true if the tag is a broad category bucket.
func IsGenericBucketTag(name string) bool {
	return genericBucketTags[NormalizeTagName(name)]
}
