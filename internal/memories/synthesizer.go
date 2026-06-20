package memories

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

const (
	// minDecisionsForSynthesis is how many closed decisions sharing an anchor
	// tag are needed before a memory is worth proposing.
	minDecisionsForSynthesis = 2
	// maxSourceIssuesPerProposal caps the provenance trail in a proposal body.
	maxSourceIssuesPerProposal = 8
	// synthesisConfidenceScale controls how quickly confidence saturates with
	// the number of supporting decisions.
	synthesisConfidenceScale = 3.0
	// synthesisSummaryMaxChars truncates each decision line in the draft body.
	synthesisSummaryMaxChars = 160

	// minIssuesForConcept is how many issues must carry a tag before it is a
	// load-bearing noun worth a canonical concept profile.
	minIssuesForConcept = 5
	// conceptTagRelevanceFloor is the minimum tag relevance for an issue to count
	// as "carrying" that tag when tallying load-bearing tags.
	conceptTagRelevanceFloor = 0.3
	// maxConceptSampleIssues caps the representative issues cited in a concept draft.
	maxConceptSampleIssues = 6
	// conceptConfidenceScale controls how quickly concept confidence saturates
	// with the number of issues carrying the tag.
	conceptConfidenceScale = 8.0
)

// decisiveClosedReasons are close reasons that record a decision even without a
// free-text note.
var decisiveClosedReasons = map[string]struct{}{
	"by_design": {},
	"wont_fix":  {},
}

// SynthesizeMemoryProposals drafts candidate memories from closed issues that
// recorded a decision, grouped by their primary anchor tag. It is pure and
// deterministic: the same corpus yields the same drafts. Tags already covered
// by an active memory or a pending proposal are skipped so repeated runs don't
// pile up duplicates. Drafts carry no embedding — enrichment happens on accept.
func SynthesizeMemoryProposals(
	closedIssues []issues.Issue,
	existingPending []domain.MemoryProposal,
	activeMemories []domain.Memory,
) []domain.MemoryProposal {
	covered := coveredAnchorTags(existingPending, activeMemories)

	byTag := make(map[string][]issues.Issue)
	for _, issue := range closedIssues {
		if !isRecordedDecision(issue) {
			continue
		}
		tag := primaryTag(issue)
		if tag == "" || covered[tag] {
			continue
		}
		byTag[tag] = append(byTag[tag], issue)
	}

	drafts := make([]domain.MemoryProposal, 0, len(byTag))
	for tag, group := range byTag {
		if len(group) < minDecisionsForSynthesis {
			continue
		}
		drafts = append(drafts, draftProposal(tag, group))
	}

	sort.SliceStable(drafts, func(i, j int) bool {
		if drafts[i].Confidence != drafts[j].Confidence {
			return drafts[i].Confidence > drafts[j].Confidence
		}
		return drafts[i].AnchorTags[0] < drafts[j].AnchorTags[0]
	})
	return drafts
}

// SynthesizeConceptProposals drafts concept memory proposals for load-bearing
// tags — nouns that recur across many issues and so deserve a canonical profile.
// It is pure and deterministic. Tags that already have an active concept or a
// pending concept proposal are skipped here; the partial unique index on
// subject_tag is the final guard at accept time, so even concurrent accepts can
// never create two active concepts for one tag. Drafts carry no embedding —
// enrichment happens on accept.
func SynthesizeConceptProposals(
	corpusIssues []issues.Issue,
	existingPending []domain.MemoryProposal,
	activeMemories []domain.Memory,
) []domain.MemoryProposal {
	covered := coveredConceptTags(existingPending, activeMemories)

	byTag := make(map[string][]issues.Issue)
	for _, issue := range corpusIssues {
		for _, tag := range issueTagSet(issue) {
			if covered[tag] {
				continue
			}
			byTag[tag] = append(byTag[tag], issue)
		}
	}

	drafts := make([]domain.MemoryProposal, 0)
	for tag, group := range byTag {
		if len(group) < minIssuesForConcept {
			continue
		}
		drafts = append(drafts, draftConceptProposal(tag, group))
	}
	sort.SliceStable(drafts, func(i, j int) bool {
		if drafts[i].Confidence != drafts[j].Confidence {
			return drafts[i].Confidence > drafts[j].Confidence
		}
		return drafts[i].SubjectTag < drafts[j].SubjectTag
	})
	return drafts
}

func draftConceptProposal(tag string, group []issues.Issue) domain.MemoryProposal {
	sort.SliceStable(group, func(i, j int) bool {
		return group[i].ID < group[j].ID
	})

	var body strings.Builder
	fmt.Fprintf(&body, "Concept profile for %q — a load-bearing tag across %d issues. "+
		"Fill in what this is, why it exists, and how it behaves. Representative issues:\n", tag, len(group))
	sourceIDs := make([]string, 0, len(group))
	for i, issue := range group {
		if i < maxConceptSampleIssues {
			fmt.Fprintf(&body, "- %s\n", truncate(strings.TrimSpace(issue.Raw), synthesisSummaryMaxChars))
		}
		sourceIDs = append(sourceIDs, issue.ID)
	}
	if len(group) > maxConceptSampleIssues {
		fmt.Fprintf(&body, "- …and %d more\n", len(group)-maxConceptSampleIssues)
	}

	confidence := math.Round((1-math.Exp(-float64(len(group))/conceptConfidenceScale))*1000) / 1000

	return domain.MemoryProposal{
		Title:          tag,
		Body:           strings.TrimRight(body.String(), "\n"),
		Kind:           domain.MemoryKindConcept,
		SubjectTag:     tag,
		AnchorTags:     []string{tag},
		SourceIssueIDs: sourceIDs,
		Confidence:     confidence,
		Status:         domain.MemoryProposalStatusPending,
		Rationale:      fmt.Sprintf("Synthesized concept for %q (load-bearing across %d issues)", tag, len(group)),
	}
}

// issueTagSet returns the normalized set of tags an issue carries: scored tags
// above the relevance floor plus any explicit tags.
func issueTagSet(issue issues.Issue) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(raw string) {
		t := domain.NormalizeTagName(raw)
		if t == "" {
			return
		}
		if _, ok := seen[t]; ok {
			return
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	for _, score := range issue.TagScores {
		if score.Relevance >= conceptTagRelevanceFloor {
			add(score.Tag)
		}
	}
	for _, tag := range issue.Tags {
		add(tag)
	}
	return out
}

func coveredConceptTags(pending []domain.MemoryProposal, active []domain.Memory) map[string]bool {
	covered := make(map[string]bool)
	for _, proposal := range pending {
		if proposal.Kind != domain.MemoryKindConcept {
			continue
		}
		if t := domain.NormalizeTagName(proposal.SubjectTag); t != "" {
			covered[t] = true
		}
	}
	for _, memory := range active {
		if memory.Kind != domain.MemoryKindConcept {
			continue
		}
		if t := domain.NormalizeTagName(memory.SubjectTag); t != "" {
			covered[t] = true
		}
	}
	return covered
}

func draftProposal(tag string, group []issues.Issue) domain.MemoryProposal {
	// Deterministic ordering of the supporting decisions.
	sort.SliceStable(group, func(i, j int) bool {
		return group[i].ID < group[j].ID
	})

	var body strings.Builder
	fmt.Fprintf(&body, "Decisions recorded across %d closed issues tagged %q:\n", len(group), tag)
	sourceIDs := make([]string, 0, len(group))
	for i, issue := range group {
		if i < maxSourceIssuesPerProposal {
			fmt.Fprintf(&body, "- %s\n", decisionSummary(issue))
		}
		sourceIDs = append(sourceIDs, issue.ID)
	}
	if len(group) > maxSourceIssuesPerProposal {
		fmt.Fprintf(&body, "- …and %d more\n", len(group)-maxSourceIssuesPerProposal)
	}

	confidence := math.Round((1-math.Exp(-float64(len(group))/synthesisConfidenceScale))*1000) / 1000

	return domain.MemoryProposal{
		Title:          fmt.Sprintf("Decisions about %s", tag),
		Body:           strings.TrimRight(body.String(), "\n"),
		Kind:           domain.MemoryKindDecision,
		AnchorTags:     []string{tag},
		SourceIssueIDs: sourceIDs,
		Confidence:     confidence,
		Status:         domain.MemoryProposalStatusPending,
		Rationale:      fmt.Sprintf("Synthesized from %d closed decisions tagged %q", len(group), tag),
	}
}

func decisionSummary(issue issues.Issue) string {
	parts := make([]string, 0, 3)
	if reason := strings.TrimSpace(issue.ClosedReason); reason != "" {
		parts = append(parts, reason)
	}
	if note := strings.TrimSpace(issue.ClosedReasonNote); note != "" {
		parts = append(parts, note)
	}
	if raw := strings.TrimSpace(issue.Raw); raw != "" {
		parts = append(parts, raw)
	}
	return truncate(strings.Join(parts, " — "), synthesisSummaryMaxChars)
}

func isRecordedDecision(issue issues.Issue) bool {
	if issue.Status != issues.StatusClosed {
		return false
	}
	if strings.TrimSpace(issue.ClosedReasonNote) != "" {
		return true
	}
	_, ok := decisiveClosedReasons[strings.ToLower(strings.TrimSpace(issue.ClosedReason))]
	return ok
}

// primaryTag returns the issue's highest-relevance tag, falling back to the
// first explicit tag. Returns "" when the issue has no usable tag.
func primaryTag(issue issues.Issue) string {
	best := ""
	bestRelevance := 0.0
	for _, score := range issue.TagScores {
		if score.Tag == "" {
			continue
		}
		if score.Relevance > bestRelevance {
			bestRelevance = score.Relevance
			best = score.Tag
		}
	}
	if best != "" {
		return domain.NormalizeTagName(best)
	}
	for _, tag := range issue.Tags {
		if t := domain.NormalizeTagName(tag); t != "" {
			return t
		}
	}
	return ""
}

// coveredAnchorTags collects the tags already covered by a decision-style memory
// or pending proposal, so decision synthesis doesn't pile up duplicates. Concepts
// are excluded: a concept ("what X is") and decisions ("choices about X") about
// the same tag are orthogonal, so a concept must not suppress decision synthesis.
func coveredAnchorTags(pending []domain.MemoryProposal, active []domain.Memory) map[string]bool {
	covered := make(map[string]bool)
	for _, proposal := range pending {
		if proposal.Kind == domain.MemoryKindConcept {
			continue
		}
		for _, tag := range proposal.AnchorTags {
			covered[domain.NormalizeTagName(tag)] = true
		}
	}
	for _, memory := range active {
		if memory.Kind == domain.MemoryKindConcept {
			continue
		}
		for _, tag := range memory.AnchorTags {
			covered[domain.NormalizeTagName(tag)] = true
		}
	}
	return covered
}

func truncate(text string, maxChars int) string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return strings.TrimSpace(string(runes[:maxChars])) + "…"
}
