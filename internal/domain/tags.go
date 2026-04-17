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
	// Evidence holds byte-offset ranges into the source text that were cited
	// by the tagger to justify this tag. Each range is resolved from a
	// verbatim quote the model returned, confirmed to appear in the source.
	// Empty means no grounded evidence was found (the model may still have
	// returned quotes that could not be matched).
	Evidence []EvidenceRange `json:"evidence,omitempty"`
}

// EvidenceRange is a half-open byte-offset range [Start, End) in the source
// text that the tagger cited to justify a tag assignment. Ranges are resolved
// from verbatim LLM-returned quotes via case- and whitespace-insensitive
// matching, so Start/End refer to positions in the original (un-normalized)
// text.
type EvidenceRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// NormalizeTagName lowercases and collapses whitespace in a tag name.
func NormalizeTagName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}
