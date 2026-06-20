package domain

import "time"

// MemoryKind categorizes what a memory captures. Memories are permanent,
// tag/region-anchored knowledge artifacts that live in the same vector space
// as issues but never decay — instead they are reinforced by related activity.
type MemoryKind string

const (
	MemoryKindDecision   MemoryKind = "decision"
	MemoryKindLesson     MemoryKind = "lesson"
	MemoryKindConstraint MemoryKind = "constraint"
	MemoryKindPattern    MemoryKind = "pattern"
	MemoryKindReference  MemoryKind = "reference"
	// MemoryKindConcept is the rich, recallable profile of a single noun — the
	// subject tag the rest of the corpus orbits. Unlike the other kinds (which
	// are predicates: a choice, a lesson, a rule), a concept's subject *is* the
	// tag. It is bound 1:1 to that tag via Memory.SubjectTag.
	MemoryKindConcept MemoryKind = "concept"
)

// MemoryStatus tracks a memory's lifecycle. Unlike issues, memories are never
// "closed"; they persist until explicitly superseded by newer knowledge or
// archived.
type MemoryStatus string

const (
	MemoryStatusActive     MemoryStatus = "active"
	MemoryStatusSuperseded MemoryStatus = "superseded"
	MemoryStatusArchived   MemoryStatus = "archived"
)

// MemorySource records whether a memory was authored by a human or synthesized
// from the corpus (matured regions, closed-issue clusters, decision operations).
type MemorySource string

const (
	MemorySourceManual      MemorySource = "manual"
	MemorySourceSynthesized MemorySource = "synthesized"
)

// Memory is a permanent knowledge artifact. It reuses the issue math
// primitives — a 1536-d embedding and a tag-relevance profile — so the same
// similarity engine can find it, but it carries a fundamentally different
// temporal model (reinforced permanence) and lifecycle.
type Memory struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Kind       MemoryKind `json:"kind"`
	AnchorTags []string   `json:"anchorTags,omitempty"`
	// SubjectTag is the single tag a concept memory profiles (kind=concept). It
	// is the 1:1 binding to the tag node: the tag stays the lightweight label,
	// this memory is its rich, recallable definition. Empty for every other kind.
	SubjectTag   string         `json:"subjectTag,omitempty"`
	AnchorRegion string         `json:"anchorRegion,omitempty"`
	TagScores    []TagRelevance `json:"tagScores"`
	Status       MemoryStatus   `json:"status"`
	SupersededBy string         `json:"supersededBy,omitempty"`
	Source       MemorySource   `json:"source"`
	// SourceIssueIDs records the issues (or decision operations) that taught us
	// this — the provenance trail behind a synthesized or promoted memory.
	SourceIssueIDs []string  `json:"sourceIssueIds,omitempty"`
	Confidence     float64   `json:"confidence"`
	CreatedBy      string    `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	// LastReinforcedAt / ReinforcementCount back the reinforced-permanence
	// model: related corpus activity strengthens a memory rather than letting
	// it decay. Populated by the reinforcement layer (Phase 2).
	LastReinforcedAt   *time.Time `json:"lastReinforcedAt,omitempty"`
	ReinforcementCount int        `json:"reinforcementCount"`
	// Embedding stays out of JSON payloads, mirroring issues.Issue.
	Embedding []float64 `json:"-"`
}

// NormalizeMemoryKind coerces a kind to a known value, defaulting to decision.
func NormalizeMemoryKind(kind MemoryKind) MemoryKind {
	switch kind {
	case MemoryKindDecision, MemoryKindLesson, MemoryKindConstraint, MemoryKindPattern, MemoryKindReference, MemoryKindConcept:
		return kind
	default:
		return MemoryKindDecision
	}
}

// NormalizeSubjectTag returns the subject tag a memory should store for its
// kind: a normalized tag name for concept memories (their 1:1 binding to a tag
// node), empty for every other kind. It does not assert presence — the service
// layer rejects a concept whose subject tag normalizes to empty.
func NormalizeSubjectTag(kind MemoryKind, subjectTag string) string {
	if NormalizeMemoryKind(kind) != MemoryKindConcept {
		return ""
	}
	return NormalizeTagName(subjectTag)
}

// NormalizeMemoryStatus coerces a status to a known value, defaulting to active.
func NormalizeMemoryStatus(status MemoryStatus) MemoryStatus {
	switch status {
	case MemoryStatusActive, MemoryStatusSuperseded, MemoryStatusArchived:
		return status
	default:
		return MemoryStatusActive
	}
}

// NormalizeMemorySource coerces a source to a known value, defaulting to manual.
func NormalizeMemorySource(source MemorySource) MemorySource {
	switch source {
	case MemorySourceManual, MemorySourceSynthesized:
		return source
	default:
		return MemorySourceManual
	}
}

// CloneMemory returns a deep copy so callers can't mutate stored slices.
func CloneMemory(m Memory) Memory {
	out := m
	if len(m.AnchorTags) > 0 {
		out.AnchorTags = append([]string(nil), m.AnchorTags...)
	}
	if len(m.TagScores) > 0 {
		out.TagScores = append([]TagRelevance(nil), m.TagScores...)
	}
	if len(m.SourceIssueIDs) > 0 {
		out.SourceIssueIDs = append([]string(nil), m.SourceIssueIDs...)
	}
	if len(m.Embedding) > 0 {
		out.Embedding = append([]float64(nil), m.Embedding...)
	}
	if m.LastReinforcedAt != nil {
		t := *m.LastReinforcedAt
		out.LastReinforcedAt = &t
	}
	return out
}
