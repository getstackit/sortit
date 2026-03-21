package issuemap

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"gonum.org/v1/gonum/mat"

	"splat/internal/issues"
	"splat/internal/scoring"
	"splat/internal/vectors"
)

type ExploreIssue struct {
	ID     string             `json:"id"`
	Raw    string             `json:"raw"`
	Status issues.IssueStatus `json:"status"`
	Tags   []TagRelevance     `json:"tags"`
}

type RelatedIssue struct {
	ID                 string             `json:"id"`
	Raw                string             `json:"raw"`
	Status             issues.IssueStatus `json:"status"`
	Tags               []TagRelevance     `json:"tags"`
	SemanticSimilarity float64            `json:"semanticSimilarity"`
	FactorSimilarity   float64            `json:"factorSimilarity"`
	CombinedSimilarity float64            `json:"combinedSimilarity"`
	Reason             string             `json:"reason"`
}

type OpportunityIssue struct {
	ID          string             `json:"id"`
	Description string             `json:"description"`
	Status      issues.IssueStatus `json:"status"`
}

type Opportunity struct {
	Title      string             `json:"title"`
	Summary    string             `json:"summary"`
	Theme      string             `json:"theme"`
	IssueIDs   []string           `json:"issueIds"`
	Issues     []OpportunityIssue `json:"issues"`
	SharedTags []string           `json:"sharedTags"`
	Confidence float64            `json:"confidence"`
	Reason     string             `json:"reason"`
}

type ExploreResponse struct {
	Issue         ExploreIssue   `json:"issue"`
	RelatedIssues []RelatedIssue `json:"relatedIssues"`
	Opportunities []Opportunity  `json:"opportunities"`
}

func ExploreFromIssuesWithTags(storeIssues []issues.Issue, storeTags []issues.Tag, targetID string, limit int) (ExploreResponse, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return ExploreResponse{}, issues.ErrNotFound
	}

	target, candidateSet, err := exploreInputIssues(storeIssues, targetID)
	if err != nil {
		return ExploreResponse{}, err
	}

	if limit <= 0 {
		limit = scoring.DefaultResultLimit
	}

	canonical, visible, boosts := deriveRelationshipSemantics(candidateSet)
	mapIssues, tagNames, issueEmbeddings, tagEmbeddings := runtimeMapInputs(candidateSet, storeTags)
	decomp := ComputeFactorDecomposition(mapIssues, tagNames, issueEmbeddings, tagEmbeddings)

	// Fall back to legacy factor vectors when decomposition didn't produce per-issue vectors.
	useDecomp := decomp.Decomposed()
	var factorVectors map[string][]float64
	if !useDecomp {
		factorVectors = runtimeFactorVectors(mapIssues, tagNames, tagEmbeddings)
	}

	targetSummary := exploreIssueSummary(target)
	now := time.Now().UTC()

	related := make([]RelatedIssue, 0, len(candidateSet)-1)
	for i := 1; i < len(candidateSet); i++ {
		candidate := candidateSet[i]
		if _, ok := visible[candidate.ID]; !ok {
			continue
		}
		candidateSummary := exploreIssueSummary(candidate)

		var factorSim, residualSim, blended float64
		if useDecomp && len(decomp.FactorEmbedding(candidate.ID)) > 0 {
			factorSim, residualSim, blended = BlendFromDecomposition(
				decomp,
				decomp.FactorEmbedding(target.ID), decomp.ResidualEmbedding(target.ID),
				decomp.FactorEmbedding(candidate.ID), decomp.ResidualEmbedding(candidate.ID),
			)
		} else {
			residualSim = vectors.UnitCosineSimilarity(issueEmbeddings[target.ID], issueEmbeddings[candidate.ID])
			factorSim = vectors.UnitCosineSimilarity(factorVectors[target.ID], factorVectors[candidate.ID])
			blended = scoring.SemanticWeight*residualSim + scoring.FactorWeight*factorSim
		}

		boost := relationshipBoost(boosts, target.ID, candidate.ID)
		authority := issues.IssueAuthority(candidate) * scoring.AuthorityConsumerWt
		combined := minFloat(1, (blended+boost+authority)*math.Sqrt(issues.IssueFreshnessWeight(candidate, now)))
		sharedTags := sharedRelevantTags(targetSummary.Tags, candidateSummary.Tags, scoring.SharedTagsLimit)

		related = append(related, RelatedIssue{
			ID:                 candidateSummary.ID,
			Raw:                candidateSummary.Raw,
			Status:             candidateSummary.Status,
			Tags:               candidateSummary.Tags,
			SemanticSimilarity: round(residualSim),
			FactorSimilarity:   round(factorSim),
			CombinedSimilarity: round(combined),
			Reason:             relatedIssueReasonWithBoost(sharedTags, residualSim, factorSim, boost, canonical[candidate.ID]),
		})
	}

	sortRelatedIssues(related)

	if len(related) > limit {
		related = related[:limit]
	}

	return ExploreResponse{
		Issue:         targetSummary,
		RelatedIssues: related,
		Opportunities: buildExploreOpportunities(targetSummary, related, maturityByIssueID(candidateSet), velocityByIssueID(candidateSet)),
	}, nil
}

func exploreInputIssues(storeIssues []issues.Issue, targetID string) (issues.Issue, []issues.Issue, error) {
	var (
		target issues.Issue
		found  bool
	)

	for _, item := range storeIssues {
		if item.ID == targetID {
			target = item
			found = true
			break
		}
	}

	if !found {
		return issues.Issue{}, nil, issues.ErrNotFound
	}

	candidateSet := make([]issues.Issue, 0, len(storeIssues))
	candidateSet = append(candidateSet, target)
	for _, item := range storeIssues {
		if item.ID == targetID || item.Status != issues.StatusOpen {
			continue
		}
		candidateSet = append(candidateSet, item)
	}

	return target, candidateSet, nil
}

func exploreIssueSummary(item issues.Issue) ExploreIssue {
	return ExploreIssue{
		ID:     item.ID,
		Raw:    item.Raw,
		Status: item.Status,
		Tags:   runtimeStoredTagRelevances(item),
	}
}

func runtimeFactorVectors(items []issues.Issue, tags []string, tagEmbeddings map[string][]float64) map[string][]float64 {
	vectors := make(map[string][]float64, len(items))
	if len(items) == 0 || len(tags) == 0 {
		return vectors
	}

	tagIndex := make(map[string]int, len(tags))
	for i, tag := range tags {
		tagIndex[tag] = i
	}

	tagCov := buildTagCovariance(tags, tagEmbeddings)
	base := make([]float64, len(tags))
	for _, item := range items {
		clear(base)
		for _, tag := range item.TagScores {
			if index, ok := tagIndex[tag.Tag]; ok {
				base[index] = tag.Relevance
			}
		}

		r := mat.NewVecDense(len(tags), base)
		var w mat.VecDense
		w.MulVec(tagCov.T(), r)
		vector := make([]float64, len(tags))
		for i := range vector {
			vector[i] = w.AtVec(i)
		}
		if !isZeroVector(vector) {
			normalizeVector(vector)
		}
		vectors[item.ID] = vector
	}

	return vectors
}

func sharedRelevantTags(a, b []TagRelevance, limit int) []string {
	if limit <= 0 || len(a) == 0 || len(b) == 0 {
		return nil
	}

	left := make(map[string]float64, len(a))
	for _, item := range a {
		left[item.Tag] = item.Relevance
	}

	type scoredTag struct {
		tag   string
		score float64
	}

	var shared []scoredTag
	for _, item := range b {
		other, ok := left[item.Tag]
		if !ok {
			continue
		}
		score := minFloat(other, item.Relevance)
		if score < 0.2 {
			continue
		}
		shared = append(shared, scoredTag{
			tag:   item.Tag,
			score: score,
		})
	}

	slices.SortFunc(shared, func(a, b scoredTag) int {
		if diff := cmp.Compare(b.score, a.score); diff != 0 {
			return diff
		}
		return cmp.Compare(a.tag, b.tag)
	})

	if len(shared) > limit {
		shared = shared[:limit]
	}

	out := make([]string, len(shared))
	for i, item := range shared {
		out[i] = item.tag
	}
	return out
}

func relatedIssueReason(sharedTags []string, semantic, factor float64) string {
	if len(sharedTags) > 0 && factor >= semantic-scoring.ReasonFactorSemanticGap {
		return "Shared factor relevance in " + strings.Join(sharedTags, ", ")
	}
	if semantic >= scoring.ReasonHighSemantic {
		return "Semantically similar language suggests a shared root cause"
	}
	if factor >= scoring.ReasonHighFactor {
		return "Similar factor profile across related tags"
	}
	return "Related by blended semantic and factor similarity"
}

func buildExploreOpportunities(target ExploreIssue, related []RelatedIssue, maturity map[string]float64, velocity map[string]float64) []Opportunity {
	if len(related) == 0 {
		return []Opportunity{}
	}

	type opportunityGroup struct {
		themeKey   string
		theme      string
		reason     string
		sharedTags []string
		issues     []RelatedIssue
	}

	order := make([]string, 0, len(related))
	groups := make(map[string]*opportunityGroup, len(related))

	for _, item := range related {
		if item.CombinedSimilarity < scoring.ExploreOpportunityMinSim {
			continue
		}

		sharedTags := sharedRelevantTags(target.Tags, item.Tags, scoring.OpportunityTagsMax)
		themeKey := "semantic"
		theme := "shared root cause"
		reason := "These issues use closely related language and may share the same fix."
		if len(sharedTags) > 0 {
			themeKey = "tags:" + strings.Join(sharedTags, "|")
			theme = strings.Join(sharedTags, " + ")
			reason = "These issues share strong factor relevance in " + strings.Join(sharedTags, ", ") + "."
		}

		group, ok := groups[themeKey]
		if !ok {
			group = &opportunityGroup{
				themeKey:   themeKey,
				theme:      theme,
				reason:     reason,
				sharedTags: sharedTags,
			}
			groups[themeKey] = group
			order = append(order, themeKey)
		}
		group.issues = append(group.issues, item)
	}

	opportunities := make([]Opportunity, 0, len(groups))
	for _, key := range order {
		group := groups[key]
		if len(group.issues) == 0 {
			continue
		}

		issueIDs := make([]string, 0, len(group.issues)+1)
		issueIDs = append(issueIDs, target.ID)
		opportunityIssues := make([]OpportunityIssue, 0, len(group.issues)+1)
		opportunityIssues = append(opportunityIssues, OpportunityIssue{
			ID:          target.ID,
			Description: target.Raw,
			Status:      target.Status,
		})

		total := 0.0
		for _, item := range group.issues {
			issueIDs = append(issueIDs, item.ID)
			opportunityIssues = append(opportunityIssues, OpportunityIssue{
				ID:          item.ID,
				Description: item.Raw,
				Status:      item.Status,
			})
			pairMaturity := (maturity[target.ID] + maturity[item.ID]) / 2
			pairVelocity := (velocity[target.ID] + velocity[item.ID]) / 2
			total += item.CombinedSimilarity * (scoring.ExploreMaturityBase + scoring.ExploreMaturityWeight*pairMaturity) * (scoring.ExploreVelocityBase + scoring.ExploreVelocityWeight*pairVelocity)
		}
		confidence := round(total / float64(len(group.issues)))

		title := "Investigate a shared root cause"
		if len(group.sharedTags) > 0 {
			title = "Solve " + strings.Join(group.sharedTags, " + ") + " issues together"
		}

		summary := "A single fix may address " + target.ID + " and " + group.issues[0].ID + "."
		if len(group.issues) > 1 {
			summary = "A single fix may address " + target.ID + " plus " + pluralizeIssueCount(len(group.issues)) + "."
		}

		opportunities = append(opportunities, Opportunity{
			Title:      title,
			Summary:    summary,
			Theme:      group.theme,
			IssueIDs:   issueIDs,
			Issues:     opportunityIssues,
			SharedTags: append([]string(nil), group.sharedTags...),
			Confidence: confidence,
			Reason:     group.reason,
		})
	}

	slices.SortFunc(opportunities, func(a, b Opportunity) int {
		if diff := cmp.Compare(b.Confidence, a.Confidence); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Title, b.Title)
	})

	return opportunities
}

func maturityByIssueID(items []issues.Issue) map[string]float64 {
	out := make(map[string]float64, len(items))
	for _, item := range items {
		out[item.ID] = issueMaturityScore(item)
	}
	return out
}

func issueMaturityScore(item issues.Issue) float64 {
	if item.LifecycleMetrics != nil && item.LifecycleMetrics.Maturity != nil {
		return minFloat(1, maxFloat(0, *item.LifecycleMetrics.Maturity))
	}
	return scoring.DefaultMaturity
}

func velocityByIssueID(items []issues.Issue) map[string]float64 {
	out := make(map[string]float64, len(items))
	for _, item := range items {
		out[item.ID] = issueVelocityScore(item)
	}
	return out
}

func issueVelocityScore(item issues.Issue) float64 {
	if item.LifecycleMetrics != nil && item.LifecycleMetrics.Velocity != nil {
		return minFloat(1, maxFloat(0, *item.LifecycleMetrics.Velocity))
	}
	return 0
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func pluralizeIssueCount(count int) string {
	if count == 1 {
		return "1 related issue"
	}
	return strconv.Itoa(count) + " related issues"
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
