package issuemap

import (
	"cmp"
	"slices"
	"strings"

	"splat/internal/issues"
)

type DerivedCorpus struct {
	Issues             []issues.Issue
	Tags               []issues.Tag
	RuntimeIssues      []issues.Issue
	TagNames           []string
	IssueEmbeddings    map[string][]float64
	TagEmbeddings      map[string][]float64
	FactorVectors      map[string][]float64
	MapIssues          []MapIssue
	Positions          map[string]Position
	AllEdges           []Edge
	Clusters           []Cluster
	IssuesByID         map[string]issues.Issue
	MapIssuesByID      map[string]MapIssue
	VisibleIssueIDs    map[string]struct{}
	CanonicalIssueIDs  map[string]string
	RelationshipBoosts map[string]map[string]float64
}

func BuildDerivedCorpus(storeIssues []issues.Issue, storeTags []issues.Tag) (DerivedCorpus, error) {
	canonical, visible, boosts := deriveRelationshipSemantics(storeIssues)

	runtimeIssues, tagNames, issueEmbeddings, tagEmbeddings := runtimeMapInputs(storeIssues, storeTags)
	factorVectors := runtimeFactorVectors(runtimeIssues, tagNames, tagEmbeddings)

	positions := make(map[string]Position, len(runtimeIssues))
	roundedPositions := make(map[string]Position, len(runtimeIssues))
	mapIssues := make([]MapIssue, len(runtimeIssues))
	mapIssuesByID := make(map[string]MapIssue, len(runtimeIssues))
	allEdges := []Edge{}
	clusters := []Cluster{}

	if len(runtimeIssues) > 0 {
		computedPositions, err := ComputePositions(runtimeIssues, tagNames, tagEmbeddings)
		if err != nil {
			return DerivedCorpus{}, err
		}
		positions = computedPositions

		for i, item := range runtimeIssues {
			position := roundPosition(positions[item.ID])
			roundedPositions[item.ID] = position
			mapIssue := MapIssue{
				ID:     item.ID,
				Raw:    storeIssues[i].Raw,
				Status: storeIssues[i].Status,
				Tags:   item.TagScores,
				X:      position.X,
				Y:      position.Y,
			}
			mapIssues[i] = mapIssue
			mapIssuesByID[item.ID] = mapIssue
		}

		allEdges = ComputeEdgesWithEmbeddings(runtimeIssues, issueEmbeddings, 0)
		sortEdgesBySimilarity(allEdges)
		clusters = ComputeFactorClusters(runtimeIssues, positions)
	}

	return DerivedCorpus{
		Issues:             cloneStoreIssues(storeIssues),
		Tags:               cloneStoreTags(storeTags),
		RuntimeIssues:      cloneRuntimeIssues(runtimeIssues),
		TagNames:           append([]string(nil), tagNames...),
		IssueEmbeddings:    cloneEmbeddingMap(issueEmbeddings),
		TagEmbeddings:      cloneEmbeddingMap(tagEmbeddings),
		FactorVectors:      cloneEmbeddingMap(factorVectors),
		MapIssues:          append([]MapIssue(nil), mapIssues...),
		Positions:          roundedPositions,
		AllEdges:           append([]Edge(nil), allEdges...),
		Clusters:           cloneClusters(clusters),
		IssuesByID:         issuesByID(storeIssues),
		MapIssuesByID:      mapIssuesByID,
		VisibleIssueIDs:    visible,
		CanonicalIssueIDs:  canonical,
		RelationshipBoosts: boosts,
	}, nil
}

func BuildMapFromCorpus(corpus DerivedCorpus, viewport *Viewport, edgeThreshold float64) (MapResponse, error) {
	base := mapBaseData{
		mapIssues:      filterVisibleMapIssues(corpus.MapIssues, corpus.VisibleIssueIDs),
		positions:      filterVisiblePositions(corpus.Positions, corpus.VisibleIssueIDs),
		candidateEdges: filterEdgesByThresholdAndVisibility(corpus.AllEdges, edgeThreshold, corpus.VisibleIssueIDs),
		clusters:       filterVisibleClusters(corpus.Clusters, corpus.VisibleIssueIDs),
	}

	edges := filterEdgesForViewport(base, normalizeViewport(viewport))
	if edges == nil {
		edges = []Edge{}
	}
	if base.clusters == nil {
		base.clusters = []Cluster{}
	}

	return MapResponse{
		Issues:   base.mapIssues,
		Edges:    edges,
		Clusters: base.clusters,
	}, nil
}

func BuildEdgeResponseFromCorpus(corpus DerivedCorpus, viewport *Viewport, edgeThreshold float64) (EdgeResponse, error) {
	base := mapBaseData{
		mapIssues:      filterVisibleMapIssues(corpus.MapIssues, corpus.VisibleIssueIDs),
		positions:      filterVisiblePositions(corpus.Positions, corpus.VisibleIssueIDs),
		candidateEdges: filterEdgesByThresholdAndVisibility(corpus.AllEdges, edgeThreshold, corpus.VisibleIssueIDs),
		clusters:       filterVisibleClusters(corpus.Clusters, corpus.VisibleIssueIDs),
	}

	edges := filterEdgesForViewport(base, normalizeViewport(viewport))
	if edges == nil {
		edges = []Edge{}
	}
	return EdgeResponse{Edges: edges}, nil
}

func SearchFromCorpus(
	corpus DerivedCorpus,
	queryRaw string,
	queryTags []issues.TagRelevance,
	queryEmbedding []float64,
	limit int,
) SearchResponse {
	queryRaw = strings.TrimSpace(queryRaw)
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	querySummary := SearchQuery{
		Raw:  queryRaw,
		Tags: searchQueryTags(queryTags),
	}

	queryVector := append([]float64(nil), queryEmbedding...)
	if isZeroVector(queryVector) {
		queryVector = runtimeIssueEmbedding(issues.Issue{
			ID:        "query",
			Raw:       queryRaw,
			TagScores: querySummary.Tags,
		}, corpus.TagEmbeddings)
	}

	queryFactor := runtimeFactorVectors([]issues.Issue{{
		ID:        "query",
		Raw:       queryRaw,
		TagScores: querySummary.Tags,
	}}, corpus.TagNames, corpus.TagEmbeddings)["query"]

	related := make([]RelatedIssue, 0, len(corpus.Issues))
	for _, candidate := range corpus.Issues {
		if _, ok := corpus.VisibleIssueIDs[candidate.ID]; !ok {
			continue
		}

		candidateSummary := exploreIssueSummary(candidate)
		semantic := cosineSimilarity(queryVector, corpus.IssueEmbeddings[candidate.ID])
		factor := cosineSimilarity(queryFactor, corpus.FactorVectors[candidate.ID])
		combined := 0.6*semantic + 0.4*factor
		sharedTags := sharedRelevantTags(querySummary.Tags, candidateSummary.Tags, 3)

		related = append(related, RelatedIssue{
			ID:                 candidateSummary.ID,
			Raw:                candidateSummary.Raw,
			Status:             candidateSummary.Status,
			Tags:               candidateSummary.Tags,
			SemanticSimilarity: round(semantic, 2),
			FactorSimilarity:   round(factor, 2),
			CombinedSimilarity: round(combined, 2),
			Reason:             relatedIssueReason(sharedTags, semantic, factor),
		})
	}

	sortRelatedIssues(related)
	if len(related) > limit {
		related = related[:limit]
	}

	return SearchResponse{
		Query:         querySummary,
		RelatedIssues: related,
	}
}

func ExploreFromCorpus(corpus DerivedCorpus, targetID string, limit int) (ExploreResponse, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return ExploreResponse{}, issues.ErrNotFound
	}

	target, ok := corpus.IssuesByID[targetID]
	if !ok {
		return ExploreResponse{}, issues.ErrNotFound
	}

	if limit <= 0 {
		limit = defaultExploreLimit
	}

	targetSummary := exploreIssueSummary(target)
	targetEmbedding := corpus.IssueEmbeddings[target.ID]
	targetFactor := corpus.FactorVectors[target.ID]

	related := make([]RelatedIssue, 0, len(corpus.Issues))
	for _, candidate := range corpus.Issues {
		if candidate.ID == targetID || candidate.Status != issues.StatusOpen {
			continue
		}
		if _, visible := corpus.VisibleIssueIDs[candidate.ID]; !visible {
			continue
		}

		candidateSummary := exploreIssueSummary(candidate)
		semantic := unitCosineSimilarity(targetEmbedding, corpus.IssueEmbeddings[candidate.ID])
		factor := unitCosineSimilarity(targetFactor, corpus.FactorVectors[candidate.ID])
		boost := relationshipBoost(corpus.RelationshipBoosts, target.ID, candidate.ID)
		combined := minFloat(1, 0.6*semantic+0.4*factor+boost)
		sharedTags := sharedRelevantTags(targetSummary.Tags, candidateSummary.Tags, 3)

		related = append(related, RelatedIssue{
			ID:                 candidateSummary.ID,
			Raw:                candidateSummary.Raw,
			Status:             candidateSummary.Status,
			Tags:               candidateSummary.Tags,
			SemanticSimilarity: round(semantic, 2),
			FactorSimilarity:   round(factor, 2),
			CombinedSimilarity: round(combined, 2),
			Reason:             relatedIssueReasonWithBoost(sharedTags, semantic, factor, boost, corpus.CanonicalIssueIDs[candidate.ID]),
		})
	}

	sortRelatedIssues(related)
	if len(related) > limit {
		related = related[:limit]
	}

	return ExploreResponse{
		Issue:         targetSummary,
		RelatedIssues: related,
		Opportunities: buildExploreOpportunities(targetSummary, related),
	}, nil
}

func deriveRelationshipSemantics(items []issues.Issue) (map[string]string, map[string]struct{}, map[string]map[string]float64) {
	linksBySource := make(map[string][]issues.IssueLink, len(items))
	canonical := make(map[string]string, len(items))
	visible := make(map[string]struct{}, len(items))
	boosts := make(map[string]map[string]float64, len(items))

	for _, item := range items {
		canonical[item.ID] = item.ID
		visible[item.ID] = struct{}{}
		for _, link := range item.Links {
			if link.SourceIssueID == item.ID {
				linksBySource[item.ID] = append(linksBySource[item.ID], link)
			}
			boost := relationshipBoostValue(link.Type)
			if boost <= 0 {
				continue
			}
			addRelationshipBoost(boosts, link.SourceIssueID, link.TargetIssueID, boost)
			addRelationshipBoost(boosts, link.TargetIssueID, link.SourceIssueID, boost)
		}
	}

	for _, item := range items {
		if target, ok := resolveCanonicalTarget(item.ID, linksBySource, map[string]bool{}); ok && target != item.ID {
			canonical[item.ID] = target
			delete(visible, item.ID)
		}
	}

	return canonical, visible, boosts
}

func resolveCanonicalTarget(id string, linksBySource map[string][]issues.IssueLink, seen map[string]bool) (string, bool) {
	if seen[id] {
		return id, false
	}
	seen[id] = true

	for _, link := range linksBySource[id] {
		if link.Type != issues.IssueLinkTypeMergedInto && link.Type != issues.IssueLinkTypeDuplicateOf {
			continue
		}
		targetID := strings.TrimSpace(link.TargetIssueID)
		if targetID == "" {
			continue
		}
		if resolved, ok := resolveCanonicalTarget(targetID, linksBySource, seen); ok {
			return resolved, true
		}
		return targetID, true
	}

	return id, true
}

func relationshipBoostValue(linkType issues.IssueLinkType) float64 {
	switch linkType {
	case issues.IssueLinkTypeRelatedTo:
		return 0.12
	case issues.IssueLinkTypeDerivedFrom:
		return 0.08
	case issues.IssueLinkTypeParentOf, issues.IssueLinkTypeChildOf:
		return 0.06
	default:
		return 0
	}
}

func addRelationshipBoost(boosts map[string]map[string]float64, sourceID, targetID string, boost float64) {
	if sourceID == "" || targetID == "" || boost <= 0 {
		return
	}
	current := boosts[sourceID]
	if current == nil {
		current = make(map[string]float64)
		boosts[sourceID] = current
	}
	if boost > current[targetID] {
		current[targetID] = boost
	}
}

func relationshipBoost(boosts map[string]map[string]float64, sourceID, targetID string) float64 {
	if boosts == nil {
		return 0
	}
	if current := boosts[sourceID]; current != nil {
		return current[targetID]
	}
	return 0
}

func relatedIssueReasonWithBoost(sharedTags []string, semantic, factor, boost float64, canonicalID string) string {
	if boost > 0.09 {
		return "Linked relationship boosts this issue beyond pure embedding similarity"
	}
	if boost > 0 {
		return "Relationship context reinforces the semantic neighborhood"
	}
	if canonicalID != "" {
		return relatedIssueReason(sharedTags, semantic, factor)
	}
	return relatedIssueReason(sharedTags, semantic, factor)
}

func sortRelatedIssues(items []RelatedIssue) {
	slices.SortFunc(items, func(a, b RelatedIssue) int {
		if diff := cmp.Compare(b.CombinedSimilarity, a.CombinedSimilarity); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.SemanticSimilarity, a.SemanticSimilarity); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.FactorSimilarity, a.FactorSimilarity); diff != 0 {
			return diff
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

func filterEdgesByThresholdAndVisibility(edges []Edge, threshold float64, visible map[string]struct{}) []Edge {
	if threshold <= 0 {
		threshold = minEdgeSimilarity
	}
	filtered := make([]Edge, 0, len(edges))
	for _, edge := range edges {
		if edge.Similarity < threshold {
			continue
		}
		if _, ok := visible[edge.Source]; !ok {
			continue
		}
		if _, ok := visible[edge.Target]; !ok {
			continue
		}
		filtered = append(filtered, edge)
	}
	return filtered
}

func filterVisibleMapIssues(items []MapIssue, visible map[string]struct{}) []MapIssue {
	filtered := make([]MapIssue, 0, len(items))
	for _, item := range items {
		if _, ok := visible[item.ID]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterVisiblePositions(items map[string]Position, visible map[string]struct{}) map[string]Position {
	filtered := make(map[string]Position, len(visible))
	for id, position := range items {
		if _, ok := visible[id]; ok {
			filtered[id] = position
		}
	}
	return filtered
}

func filterVisibleClusters(items []Cluster, visible map[string]struct{}) []Cluster {
	filtered := make([]Cluster, 0, len(items))
	for _, cluster := range items {
		ids := make([]string, 0, len(cluster.IssueIDs))
		for _, id := range cluster.IssueIDs {
			if _, ok := visible[id]; ok {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		cluster.IssueIDs = ids
		filtered = append(filtered, cluster)
	}
	return filtered
}

func cloneEmbeddingMap(input map[string][]float64) map[string][]float64 {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string][]float64, len(input))
	for key, value := range input {
		out[key] = append([]float64(nil), value...)
	}
	return out
}

func issuesByID(items []issues.Issue) map[string]issues.Issue {
	index := make(map[string]issues.Issue, len(items))
	for _, item := range items {
		index[item.ID] = item
	}
	return index
}

func cloneStoreIssues(items []issues.Issue) []issues.Issue {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]issues.Issue, len(items))
	for i, item := range items {
		cloned[i] = item
		cloned[i].Tags = append([]string(nil), item.Tags...)
		cloned[i].Discussion = append([]issues.IssuePost(nil), item.Discussion...)
		cloned[i].Links = append([]issues.IssueLink(nil), item.Links...)
		cloned[i].Operations = append([]issues.IssueOperation(nil), item.Operations...)
		cloned[i].TagScores = append([]issues.TagRelevance(nil), item.TagScores...)
		cloned[i].Embedding = append([]float64(nil), item.Embedding...)
	}
	return cloned
}

func cloneStoreTags(items []issues.Tag) []issues.Tag {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]issues.Tag, len(items))
	for i, item := range items {
		cloned[i] = item
		cloned[i].Embedding = append([]float64(nil), item.Embedding...)
	}
	return cloned
}

func cloneRuntimeIssues(items []issues.Issue) []issues.Issue {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]issues.Issue, len(items))
	for i, item := range items {
		cloned[i] = issues.Issue{
			ID:        item.ID,
			Raw:       item.Raw,
			Status:    item.Status,
			TagScores: append([]TagRelevance(nil), item.TagScores...),
			Embedding: append([]float64(nil), item.Embedding...),
		}
	}
	return cloned
}

func cloneClusters(items []Cluster) []Cluster {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]Cluster, len(items))
	for i, item := range items {
		cloned[i] = Cluster{
			Label:    item.Label,
			CenterX:  item.CenterX,
			CenterY:  item.CenterY,
			Radius:   item.Radius,
			Color:    item.Color,
			IssueIDs: append([]string(nil), item.IssueIDs...),
			TopTag:   item.TopTag,
		}
	}
	return cloned
}
