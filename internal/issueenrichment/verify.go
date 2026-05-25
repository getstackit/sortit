package enrichment

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"sortit/internal/ai"
	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/tags"
	"sortit/internal/vectors"
)

const (
	genericSpecificityThreshold = 0.3
	genericMultiplier           = 0.6
	defaultSpecificity          = 0.5
	issueTagRelevanceFloor      = 0.08
	verifierWeakAlignment       = 0.16
	verifierFlaggedAlignment    = 0.08
	verifierDominatingAlignment = 0.35
	verifierDominanceMargin     = 0.18
	verifierSpecificityMargin   = 0.10
	verifierDownrankMultiplier  = 0.75
	// analyzerNegationMinConfidence is the floor below which an analyzer-emitted
	// negation is discarded even if it has resolvable evidence. Negative signal
	// requires both textual grounding and meaningful confidence.
	analyzerNegationMinConfidence = 0.1
	// negationConfidenceCap mirrors the analyzer-side cap so the verifier never
	// persists negation values above 0.7.
	negationConfidenceCap = 0.7
)

func attenuateGenericScores(scores []issues.TagRelevance, tagSpecificity map[string]*float64) []issues.TagRelevance {
	if len(scores) == 0 {
		return scores
	}

	hasSpecific := false
	for _, score := range scores {
		if tagSpecificityValue(score.Tag, tagSpecificity) >= genericSpecificityThreshold && score.Relevance > 0 {
			hasSpecific = true
			break
		}
	}
	if !hasSpecific {
		return scores
	}

	out := make([]issues.TagRelevance, len(scores))
	for i, score := range scores {
		out[i] = score
		if tagSpecificityValue(score.Tag, tagSpecificity) < genericSpecificityThreshold {
			out[i].Relevance = score.Relevance * genericMultiplier
		}
	}
	return out
}

func tagSpecificityValue(name string, specificity map[string]*float64) float64 {
	if specificity == nil {
		return defaultSpecificity
	}
	value, ok := specificity[name]
	if !ok || value == nil {
		return defaultSpecificity
	}
	return *value
}

type verifierCandidate struct {
	Name        string
	Alignment   float64
	Specificity float64
	Sources     []string
}

func (s *IssueEnricher) decorateAndVerifyTagScores(
	ctx context.Context,
	rawText string,
	issueEmbedding []float64,
	candidates tags.CandidateTaxonomy,
	scores []issues.TagRelevance,
	aiScores []ai.TagScore,
	aiNegated []ai.NegatedTag,
	tagSpecificity map[string]*float64,
	verify bool,
) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}

	evidenceByTag := make(map[string][]string, len(aiScores))
	for _, score := range aiScores {
		name := normalizeTagName(score.Tag)
		if name != "" && len(score.Evidence) > 0 {
			evidenceByTag[name] = score.Evidence
		}
	}

	candidateByName := make(map[string]tags.CandidateTag, len(candidates.Tags))
	for _, candidate := range candidates.Tags {
		name := normalizeTagName(candidate.Name)
		if name == "" {
			continue
		}
		candidateByName[name] = candidate
	}

	storedTags, err := s.catalog.StoredTags(ctx)
	if err != nil {
		storedTags = nil
	}
	tagEmbeddingByName := make(map[string][]float64, len(storedTags))
	for _, tag := range storedTags {
		name := normalizeTagName(tag.Name)
		if name == "" || len(tag.Embedding) == 0 {
			continue
		}
		tagEmbeddingByName[name] = append([]float64(nil), tag.Embedding...)
	}

	assigned := make(map[string]struct{}, len(scores))
	for _, score := range scores {
		assigned[normalizeTagName(score.Tag)] = struct{}{}
	}

	unassigned := make([]verifierCandidate, 0, len(candidates.Tags))
	if len(issueEmbedding) > 0 {
		for _, candidate := range candidates.Tags {
			name := normalizeTagName(candidate.Name)
			if name == "" {
				continue
			}
			if _, ok := assigned[name]; ok {
				continue
			}
			embedding := tagEmbeddingByName[name]
			if len(embedding) == 0 {
				continue
			}
			alignment := roundVerifierMetric(vectors.CosineSimilarity(issueEmbedding, embedding))
			unassigned = append(unassigned, verifierCandidate{
				Name:        name,
				Alignment:   alignment,
				Specificity: tagSpecificityValue(name, tagSpecificity),
				Sources:     candidateSourceStrings(candidate.Sources),
			})
		}
		slices.SortFunc(unassigned, func(a, b verifierCandidate) int {
			if c := cmp.Compare(b.Alignment, a.Alignment); c != 0 {
				return c
			}
			if c := cmp.Compare(b.Specificity, a.Specificity); c != 0 {
				return c
			}
			return cmp.Compare(a.Name, b.Name)
		})
	}

	out := issuesDisplayCopyTagScores(scores)
	for i := range out {
		name := normalizeTagName(out[i].Tag)
		if candidate, ok := candidateByName[name]; ok {
			out[i].CandidateSources = candidateSourceStrings(candidate.Sources)
		}
		if specificity, ok := tagSpecificity[name]; ok && specificity != nil {
			out[i].Specificity = cloneMetricPointer(*specificity)
		}
		if len(issueEmbedding) > 0 {
			if embedding := tagEmbeddingByName[name]; len(embedding) > 0 {
				out[i].Alignment = cloneMetricPointer(vectors.CosineSimilarity(issueEmbedding, embedding))
			}
		}

		if quotes, ok := evidenceByTag[name]; ok && rawText != "" {
			out[i].Evidence = resolveEvidenceRanges(rawText, quotes)
		}
		hasGroundedEvidence := len(out[i].Evidence) > 0

		if !verify {
			continue
		}

		assignedSpecificity := tagSpecificityValue(name, tagSpecificity)
		dominating, gap := bestDominatingCandidate(out[i], assignedSpecificity, unassigned)
		if dominating != nil {
			out[i].DominatedBy = dominating.Name
			out[i].DominanceGap = cloneMetricPointer(gap)
		}

		switch {
		case hasGroundedEvidence && out[i].Alignment != nil && *out[i].Alignment < verifierFlaggedAlignment && out[i].Relevance >= 0.4:
			out[i].VerificationVerdict = domain.TagVerificationVerdictKeep
			out[i].VerificationReason = "weak alignment rescued by grounded source-text evidence"
		case out[i].Alignment != nil && *out[i].Alignment < verifierFlaggedAlignment && out[i].Relevance >= 0.4:
			out[i].VerificationVerdict = domain.TagVerificationVerdictFlagged
			if dominating != nil {
				out[i].VerificationReason = fmt.Sprintf("very weak alignment; nearby unassigned %s is better aligned", dominating.Name)
			} else {
				out[i].VerificationReason = "high relevance but very weak embedding alignment"
			}
		case hasGroundedEvidence && dominating != nil && out[i].Alignment != nil && *out[i].Alignment < verifierWeakAlignment:
			out[i].VerificationVerdict = domain.TagVerificationVerdictKeep
			out[i].VerificationReason = fmt.Sprintf("weak alignment rescued by grounded evidence despite stronger nearby %s", dominating.Name)
		case dominating != nil && out[i].Alignment != nil && *out[i].Alignment < verifierWeakAlignment:
			out[i].VerificationVerdict = domain.TagVerificationVerdictDownRank
			out[i].VerificationReason = fmt.Sprintf("dominated by nearby unassigned %s", dominating.Name)
			out[i].Relevance = roundRelevance(max(issueTagRelevanceFloor, out[i].Relevance*verifierDownrankMultiplier))
		case anchorOnlyCandidate(out[i].CandidateSources) && out[i].Alignment != nil && *out[i].Alignment < verifierWeakAlignment && out[i].Relevance >= 0.35:
			out[i].VerificationVerdict = domain.TagVerificationVerdictFlagged
			out[i].VerificationReason = "anchor-only candidate with weak embedding alignment"
		default:
			out[i].VerificationVerdict = domain.TagVerificationVerdictKeep
			if hasGroundedEvidence {
				out[i].VerificationReason = "grounded by source-text evidence"
			} else if dominating != nil {
				out[i].VerificationReason = fmt.Sprintf("kept despite stronger nearby %s", dominating.Name)
			}
		}
	}

	out = applyAnalyzerNegations(out, aiNegated, rawText, candidateByName, tagSpecificity, tagEmbeddingByName, issueEmbedding)

	slices.SortStableFunc(out, func(a, b issues.TagRelevance) int {
		if c := cmp.Compare(b.Relevance, a.Relevance); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})
	return out
}

// applyAnalyzerNegations cross-checks each analyzer-emitted negation against
// the source text. Negations whose evidence quotes cannot be located in the
// raw text are discarded silently — the bar for negative signal is higher
// than for positive. Surviving negations are written onto the matching tag
// row, or appended as synthetic Relevance:0 rows when the analyzer negates a
// tag it did not positively assign. Synthetic rows still receive the same
// decoration (candidate sources, specificity, alignment) that other rows do
// so downstream consumers can reason about them uniformly.
func applyAnalyzerNegations(
	out []issues.TagRelevance,
	aiNegated []ai.NegatedTag,
	rawText string,
	candidateByName map[string]tags.CandidateTag,
	tagSpecificity map[string]*float64,
	tagEmbeddingByName map[string][]float64,
	issueEmbedding []float64,
) []issues.TagRelevance {
	if len(aiNegated) == 0 || rawText == "" {
		return out
	}

	indexByTag := make(map[string]int, len(out))
	for i, row := range out {
		indexByTag[normalizeTagName(row.Tag)] = i
	}

	for _, negation := range aiNegated {
		if negation.Confidence < analyzerNegationMinConfidence {
			continue
		}
		ranges := resolveEvidenceRanges(rawText, negation.Evidence)
		if len(ranges) == 0 {
			continue
		}
		name := normalizeTagName(negation.Tag)
		if name == "" {
			continue
		}
		confidence := min(negationConfidenceCap, negation.Confidence)
		confidence = roundVerifierMetric(confidence)
		reason := "explicitly refuted by source text"

		if idx, ok := indexByTag[name]; ok {
			// Analyzer evidence overrides any prior negation source (e.g. a
			// future verifier-dominance negation in PR 5) when both target
			// the same tag.
			out[idx].Negation = cloneMetricPointer(confidence)
			out[idx].NegationProvenance = domain.NegationProvenanceAnalyzer
			out[idx].NegationEvidence = append([]domain.EvidenceRange(nil), ranges...)
			out[idx].NegationReason = reason
			continue
		}

		synthetic := issues.TagRelevance{
			Tag:                negation.Tag,
			Relevance:          0,
			Negation:           cloneMetricPointer(confidence),
			NegationProvenance: domain.NegationProvenanceAnalyzer,
			NegationEvidence:   append([]domain.EvidenceRange(nil), ranges...),
			NegationReason:     reason,
		}
		if candidate, ok := candidateByName[name]; ok {
			synthetic.CandidateSources = candidateSourceStrings(candidate.Sources)
		}
		if specificity, ok := tagSpecificity[name]; ok && specificity != nil {
			synthetic.Specificity = cloneMetricPointer(*specificity)
		}
		if len(issueEmbedding) > 0 {
			if embedding := tagEmbeddingByName[name]; len(embedding) > 0 {
				synthetic.Alignment = cloneMetricPointer(vectors.CosineSimilarity(issueEmbedding, embedding))
			}
		}
		out = append(out, synthetic)
		indexByTag[name] = len(out) - 1
	}

	return out
}

func bestDominatingCandidate(
	score issues.TagRelevance,
	assignedSpecificity float64,
	unassigned []verifierCandidate,
) (*verifierCandidate, float64) {
	if score.Alignment == nil {
		return nil, 0
	}
	bestIndex := -1
	bestGap := 0.0
	for i, candidate := range unassigned {
		gap := candidate.Alignment - *score.Alignment
		if candidate.Alignment < verifierDominatingAlignment || gap < verifierDominanceMargin {
			continue
		}
		if candidate.Specificity+0.001 < assignedSpecificity+verifierSpecificityMargin &&
			assignedSpecificity >= genericSpecificityThreshold &&
			!anchorOnlyCandidate(score.CandidateSources) {
			continue
		}
		if bestIndex == -1 ||
			gap > bestGap ||
			(gap == bestGap && candidate.Specificity > unassigned[bestIndex].Specificity) {
			bestIndex = i
			bestGap = gap
		}
	}
	if bestIndex == -1 {
		return nil, 0
	}
	return &unassigned[bestIndex], roundVerifierMetric(bestGap)
}

func candidateSourceStrings(sources []tags.CandidateSource) []string {
	if len(sources) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(sources))
	out := make([]string, 0, len(sources))
	for _, source := range []tags.CandidateSource{
		tags.CandidateSourceExplicit,
		tags.CandidateSourceRetrieval,
		tags.CandidateSourceAnchor,
		tags.CandidateSourceFullCatalog,
	} {
		for _, candidate := range sources {
			if candidate != source {
				continue
			}
			label := string(candidate)
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			out = append(out, label)
		}
	}
	for _, source := range sources {
		label := string(source)
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func anchorOnlyCandidate(sources []string) bool {
	return len(sources) == 1 && sources[0] == string(tags.CandidateSourceAnchor)
}

func issuesDisplayCopyTagScores(scores []issues.TagRelevance) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}
	out := make([]issues.TagRelevance, len(scores))
	for i, score := range scores {
		out[i] = score
		out[i].CandidateSources = append([]string(nil), score.CandidateSources...)
		out[i].Alignment = copyMetricPointer(score.Alignment)
		out[i].Specificity = copyMetricPointer(score.Specificity)
		out[i].DominanceGap = copyMetricPointer(score.DominanceGap)
		out[i].Evidence = append([]domain.EvidenceRange(nil), score.Evidence...)
	}
	return out
}

type normMapping struct {
	text    string
	offsets []int
}

func normalizeWithOffsets(raw string) normMapping {
	var b strings.Builder
	b.Grow(len(raw))
	offsets := make([]int, 0, len(raw))
	prevSpace := false
	for i, r := range raw {
		switch r {
		case '\u2018', '\u2019', '\u201A', '\u201B':
			r = '\''
		case '\u201C', '\u201D', '\u201E', '\u201F':
			r = '"'
		case '\u2013', '\u2014':
			r = '-'
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' {
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				offsets = append(offsets, i)
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		lower := unicode.ToLower(r)
		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], lower)
		for j := 0; j < n; j++ {
			b.WriteByte(buf[j])
			offsets = append(offsets, i)
		}
	}
	s := strings.TrimRight(b.String(), " ")
	if len(s) < b.Len() {
		offsets = offsets[:len(s)]
	}
	trimmed := strings.TrimLeft(s, " ")
	if delta := len(s) - len(trimmed); delta > 0 {
		offsets = offsets[delta:]
		s = trimmed
	}
	return normMapping{text: s, offsets: offsets}
}

func normalizeQuote(quote string) string {
	return normalizeWithOffsets(quote).text
}

func resolveEvidenceRanges(rawText string, quotes []string) []domain.EvidenceRange {
	if rawText == "" || len(quotes) == 0 {
		return nil
	}
	nm := normalizeWithOffsets(rawText)
	var out []domain.EvidenceRange
	for _, quote := range quotes {
		normQuote := normalizeQuote(strings.TrimSpace(quote))
		if normQuote == "" {
			continue
		}
		idx := strings.Index(nm.text, normQuote)
		if idx < 0 {
			continue
		}
		start := nm.offsets[idx]
		endNorm := idx + len(normQuote) - 1
		lastOrigPos := nm.offsets[endNorm]
		_, runeSize := utf8.DecodeRuneInString(rawText[lastOrigPos:])
		end := lastOrigPos + runeSize
		out = append(out, domain.EvidenceRange{
			Start:  start,
			End:    end,
			Text:   rawText[start:end],
			Source: "source_text",
			Kind:   "direct_quote",
		})
	}
	return out
}

func cloneMetricPointer(value float64) *float64 {
	rounded := roundVerifierMetric(value)
	return &rounded
}

func copyMetricPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return cloneMetricPointer(*value)
}

func roundVerifierMetric(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func roundRelevance(value float64) float64 {
	return math.Round(value*100) / 100
}
