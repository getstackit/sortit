package enrichment

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"

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
	issueEmbedding []float64,
	candidates tags.CandidateTaxonomy,
	scores []issues.TagRelevance,
	tagSpecificity map[string]*float64,
	verify bool,
) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
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
		case out[i].Alignment != nil && *out[i].Alignment < verifierFlaggedAlignment && out[i].Relevance >= 0.4:
			out[i].VerificationVerdict = domain.TagVerificationVerdictFlagged
			if dominating != nil {
				out[i].VerificationReason = fmt.Sprintf("very weak alignment; nearby unassigned %s is better aligned", dominating.Name)
			} else {
				out[i].VerificationReason = "high relevance but very weak embedding alignment"
			}
		case dominating != nil && out[i].Alignment != nil && *out[i].Alignment < verifierWeakAlignment:
			out[i].VerificationVerdict = domain.TagVerificationVerdictDownRank
			out[i].VerificationReason = fmt.Sprintf("dominated by nearby unassigned %s", dominating.Name)
			out[i].Relevance = roundRelevance(max(issueTagRelevanceFloor, out[i].Relevance*verifierDownrankMultiplier))
		case anchorOnlyCandidate(out[i].CandidateSources) && out[i].Alignment != nil && *out[i].Alignment < verifierWeakAlignment && out[i].Relevance >= 0.35:
			out[i].VerificationVerdict = domain.TagVerificationVerdictFlagged
			out[i].VerificationReason = "anchor-only candidate with weak embedding alignment"
		default:
			out[i].VerificationVerdict = domain.TagVerificationVerdictKeep
			if dominating != nil {
				out[i].VerificationReason = fmt.Sprintf("kept despite stronger nearby %s", dominating.Name)
			}
		}
	}

	slices.SortStableFunc(out, func(a, b issues.TagRelevance) int {
		if c := cmp.Compare(b.Relevance, a.Relevance); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})
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
