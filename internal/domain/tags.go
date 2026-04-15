package domain

import "strings"

type TagVerificationVerdict string

const (
	TagVerificationVerdictKeep     TagVerificationVerdict = "keep"
	TagVerificationVerdictDownRank TagVerificationVerdict = "down-rank"
	TagVerificationVerdictFlagged  TagVerificationVerdict = "flagged"
)

// TagRelevance represents the relevance score of a tag to an issue.
type TagRelevance struct {
	Tag                 string                 `json:"tag"`
	Relevance           float64                `json:"relevance"`
	Suggested           bool                   `json:"suggested,omitempty"`
	Description         string                 `json:"description,omitempty"`
	CandidateSources    []string               `json:"candidateSources,omitempty"`
	Alignment           *float64               `json:"alignment,omitempty"`
	Specificity         *float64               `json:"specificity,omitempty"`
	VerificationVerdict TagVerificationVerdict `json:"verificationVerdict,omitempty"`
	VerificationReason  string                 `json:"verificationReason,omitempty"`
	DominatedBy         string                 `json:"dominatedBy,omitempty"`
	DominanceGap        *float64               `json:"dominanceGap,omitempty"`
	// Evidence holds verbatim quotes from the source text that the tagger
	// claims justify this tag. Quotes are preserved as-returned by the model.
	Evidence []string `json:"evidence,omitempty"`
	// EvidenceMatched counts how many entries in Evidence were confirmed to
	// appear (case- and whitespace-insensitively) in the source text.
	// nil means the evidence check did not run for this tag.
	EvidenceMatched *int `json:"evidenceMatched,omitempty"`
}

// NormalizeTagName lowercases and collapses whitespace in a tag name.
func NormalizeTagName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}
