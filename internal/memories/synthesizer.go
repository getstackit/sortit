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

func coveredAnchorTags(pending []domain.MemoryProposal, active []domain.Memory) map[string]bool {
	covered := make(map[string]bool)
	for _, proposal := range pending {
		for _, tag := range proposal.AnchorTags {
			covered[domain.NormalizeTagName(tag)] = true
		}
	}
	for _, memory := range active {
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
