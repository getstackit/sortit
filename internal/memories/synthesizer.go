package memories

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"sortit/internal/domain"
	"sortit/internal/issuemath"
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
	// conceptSpecificityFloor is the minimum tag specificity for a tag to be a
	// concept subject. Generic bucket tags (improvement, backend, ui, …) score
	// well below this and are frequent but not meaningful nouns, so they are
	// skipped. Tags whose specificity is unknown are not gated (frequency already
	// flags them as load-bearing). Tunable; see the calibration in the design.
	conceptSpecificityFloor = 0.2
	// maxConceptSampleIssues caps the representative issues cited in a concept draft.
	maxConceptSampleIssues = 6
	// conceptConfidenceScale controls how quickly concept confidence saturates
	// with the number of issues carrying the tag.
	conceptConfidenceScale = 8.0

	// residualClusterMinSize is the minimum number of issues sharing an
	// unexplained residual before the cluster is worth proposing a concept for.
	// Below this, a shared residual is more likely noise than a real missing noun.
	residualClusterMinSize = 3
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
// It is pure and deterministic. A tag must be both frequent (>= minIssuesForConcept
// issues) and specific (specificity >= conceptSpecificityFloor) to qualify, so
// generic bucket tags don't become concepts. tagSpecificity maps normalized tag
// names to their catalog specificity; tags absent from it are not gated. Tags
// that already have an active concept or a pending concept proposal are skipped;
// the partial unique index on subject_tag is the final guard at accept time.
// Drafts carry no embedding — enrichment happens on accept.
func SynthesizeConceptProposals(
	corpusIssues []issues.Issue,
	existingPending []domain.MemoryProposal,
	activeMemories []domain.Memory,
	tagSpecificity map[string]float64,
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
		if spec, ok := tagSpecificity[tag]; ok && spec < conceptSpecificityFloor {
			continue // generic bucket tag — frequent but not a meaningful noun
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

// SynthesizeResidualConceptProposals mines clusters of issues that share an
// unexplained embedding residual — the concept their tags are failing to capture
// — and drafts a concept proposal per cluster. It is pure and deterministic, but
// produces UNNAMED concept drafts (SubjectTag empty, SourceIssueIDs = the cluster
// members): naming and profiling the invented concept require the LLM and happen
// at the service layer (ProposeConceptFromCluster). Issues already cited as the
// provenance of an active concept or pending concept proposal are excluded from
// the seed pool so reruns don't re-mine covered ground. The embeddings must
// already be centered (issuemath.CenterEmbeddings), matching the debug R² path.
func SynthesizeResidualConceptProposals(
	corpusIssues []issues.Issue,
	tagNames []string,
	issueEmbeddings map[string][]float64,
	tagEmbeddings map[string][]float64,
	existingPending []domain.MemoryProposal,
	activeMemories []domain.Memory,
) []domain.MemoryProposal {
	decomp := issuemath.ComputeFactorDecomposition(corpusIssues, tagNames, issueEmbeddings, tagEmbeddings)
	if !decomp.Decomposed() {
		return nil
	}

	covered := coveredConceptSourceIssues(existingPending, activeMemories)

	// Seed pool: poorly-explained issues (low R²) not already covered. Sorted by
	// R² ascending (worst-explained first) then ID, so clustering is deterministic
	// and the least-explained issues anchor clusters.
	type seed struct {
		id string
		r2 float64
	}
	seeds := make([]seed, 0)
	candidate := make(map[string]bool)
	for _, issue := range corpusIssues {
		if covered[issue.ID] {
			continue
		}
		r2, ok := decomp.IssueR2(issue.ID)
		if !ok || r2 >= issuemath.ResidualLowR2Threshold {
			continue
		}
		if len(decomp.ResidualEmbedding(issue.ID)) == 0 {
			continue
		}
		seeds = append(seeds, seed{id: issue.ID, r2: r2})
		candidate[issue.ID] = true
	}
	sort.SliceStable(seeds, func(i, j int) bool {
		if seeds[i].r2 != seeds[j].r2 {
			return seeds[i].r2 < seeds[j].r2
		}
		return seeds[i].id < seeds[j].id
	})

	// Greedy single-link clustering over the residual directions of the seed pool.
	// An issue is only marked consumed once its cluster is actually emitted, so a
	// seed whose cluster is too small can still join a later, larger one.
	clustered := make(map[string]bool)
	drafts := make([]domain.MemoryProposal, 0)
	for _, s := range seeds {
		if clustered[s.id] {
			continue
		}
		cluster := []string{s.id}
		for _, neighbor := range decomp.ResidualNeighbors(s.id, issuemath.ResidualNeighborSimThreshold) {
			if !candidate[neighbor.ID] || clustered[neighbor.ID] {
				continue
			}
			cluster = append(cluster, neighbor.ID)
		}
		if len(cluster) < residualClusterMinSize {
			continue
		}
		for _, id := range cluster {
			clustered[id] = true
		}
		sort.Strings(cluster)
		drafts = append(drafts, draftResidualConceptProposal(cluster))
	}
	return drafts
}

// draftResidualConceptProposal builds an unnamed concept proposal for a residual
// cluster. Title/Body/SubjectTag are left empty for the AI proposer to fill; the
// pure synthesizer only knows the cluster membership and its size-driven
// confidence.
func draftResidualConceptProposal(sourceIDs []string) domain.MemoryProposal {
	confidence := math.Round((1-math.Exp(-float64(len(sourceIDs))/conceptConfidenceScale))*1000) / 1000
	return domain.MemoryProposal{
		Kind:           domain.MemoryKindConcept,
		SourceIssueIDs: sourceIDs,
		Confidence:     confidence,
		Status:         domain.MemoryProposalStatusPending,
		Rationale: fmt.Sprintf(
			"Synthesized from a residual cluster of %d issues sharing a concept the existing tags miss",
			len(sourceIDs),
		),
	}
}

// coveredConceptSourceIssues collects the issue IDs already cited as the
// provenance of an active concept or pending concept proposal, so residual mining
// doesn't re-propose a concept around already-conceptualized issues.
func coveredConceptSourceIssues(pending []domain.MemoryProposal, active []domain.Memory) map[string]bool {
	covered := make(map[string]bool)
	for _, proposal := range pending {
		if proposal.Kind != domain.MemoryKindConcept {
			continue
		}
		for _, id := range proposal.SourceIssueIDs {
			if id = strings.TrimSpace(id); id != "" {
				covered[id] = true
			}
		}
	}
	for _, memory := range active {
		if memory.Kind != domain.MemoryKindConcept {
			continue
		}
		for _, id := range memory.SourceIssueIDs {
			if id = strings.TrimSpace(id); id != "" {
				covered[id] = true
			}
		}
	}
	return covered
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
