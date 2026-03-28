package issuemap

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"splat/internal/issueanalytics"
	"splat/internal/issuemath"
	"splat/internal/issues"
)

type BuildMapProjectionStep struct {
	Name     string
	Duration time.Duration
}

type BuildMapProjectionProfile struct {
	IssueCount        int
	StoredTagCount    int
	RuntimeIssueCount int
	TagNameCount      int
	VisibleIssueCount int
	EdgeCount         int
	ClusterCount      int
	Steps             []BuildMapProjectionStep
	TotalDuration     time.Duration
}

type MapProjection struct {
	Available         bool
	UnavailableReason string
	IssueCount        int
	MinimumIssueCount int
	MapIssues         []MapIssue
	AllEdges          []Edge
	Clusters          []Cluster
	VisibleIssueIDs   map[string]struct{}
}

func SubsetMapProjection(projection MapProjection, issueIDs map[string]struct{}) MapProjection {
	if len(issueIDs) == 0 {
		return MapProjection{
			Available:         false,
			UnavailableReason: mapUnavailableReasonInsufficientIssueCount,
			IssueCount:        0,
			MinimumIssueCount: projection.MinimumIssueCount,
			VisibleIssueIDs:   map[string]struct{}{},
		}
	}

	if len(issueIDs) == len(projection.MapIssues) {
		return projection
	}

	filteredVisible := make(map[string]struct{}, len(issueIDs))
	for id := range issueIDs {
		if _, ok := projection.VisibleIssueIDs[id]; ok {
			filteredVisible[id] = struct{}{}
		}
	}

	return MapProjection{
		Available:         projection.Available,
		UnavailableReason: projection.UnavailableReason,
		IssueCount:        len(issueIDs),
		MinimumIssueCount: projection.MinimumIssueCount,
		MapIssues:         filterMapIssuesByID(projection.MapIssues, issueIDs),
		AllEdges:          filterEdgesByID(projection.AllEdges, issueIDs),
		Clusters:          filterVisibleClusters(projection.Clusters, issueIDs),
		VisibleIssueIDs:   filteredVisible,
	}
}

func BuildMapProjection(storeIssues []issues.MapProjectionIssue, storeTags []issues.Tag) (MapProjection, error) {
	projection, _, err := BuildMapProjectionProfiled(storeIssues, storeTags)
	return projection, err
}

func BuildMapProjectionProfiled(
	storeIssues []issues.MapProjectionIssue,
	storeTags []issues.Tag,
) (MapProjection, BuildMapProjectionProfile, error) {
	startedAt := time.Now()
	profile := BuildMapProjectionProfile{
		IssueCount:     len(storeIssues),
		StoredTagCount: len(storeTags),
	}

	stepStartedAt := time.Now()
	_, visible, _ := deriveProjectionRelationshipSemantics(storeIssues)
	profile.VisibleIssueCount = len(visible)
	profile.Steps = append(profile.Steps, BuildMapProjectionStep{
		Name:     "derive_relationship_semantics",
		Duration: time.Since(stepStartedAt),
	})

	stepStartedAt = time.Now()
	runtimeIssues, tagNames, issueEmbeddings, tagEmbeddings := runtimeProjectionInputs(storeIssues, storeTags)
	profile.RuntimeIssueCount = len(runtimeIssues)
	profile.TagNameCount = len(tagNames)
	profile.Steps = append(profile.Steps, BuildMapProjectionStep{
		Name:     "runtime_map_inputs",
		Duration: time.Since(stepStartedAt),
	})

	stepStartedAt = time.Now()
	_ = runtimeFactorVectors(runtimeIssues, tagNames, tagEmbeddings)
	profile.Steps = append(profile.Steps, BuildMapProjectionStep{
		Name:     "runtime_factor_vectors",
		Duration: time.Since(stepStartedAt),
	})

	unavailableReason := mapProjectionUnavailableReason(len(runtimeIssues), len(tagNames))
	if unavailableReason != "" {
		profile.TotalDuration = time.Since(startedAt)
		return MapProjection{
			Available:         false,
			UnavailableReason: unavailableReason,
			IssueCount:        len(runtimeIssues),
			MinimumIssueCount: minMapIssueCount,
			MapIssues:         []MapIssue{},
			AllEdges:          []Edge{},
			Clusters:          []Cluster{},
			VisibleIssueIDs:   map[string]struct{}{},
		}, profile, nil
	}

	var positions map[string]Position
	roundedPositions := make(map[string]Position, len(runtimeIssues))
	mapIssues := make([]MapIssue, len(runtimeIssues))
	allEdges := []Edge{}
	clusters := []Cluster{}

	if len(runtimeIssues) > 0 {
		stepStartedAt = time.Now()
		computedPositions, err := issuemath.ComputePositions(runtimeIssues, tagNames, tagEmbeddings)
		if err != nil {
			return MapProjection{}, BuildMapProjectionProfile{}, err
		}
		positions = computedPositions
		profile.Steps = append(profile.Steps, BuildMapProjectionStep{
			Name:     "compute_positions",
			Duration: time.Since(stepStartedAt),
		})

		stepStartedAt = time.Now()
		for i, item := range runtimeIssues {
			position := roundPosition(positions[item.ID])
			roundedPositions[item.ID] = position
			mapIssue := MapIssue{
				ID:         item.ID,
				Raw:        storeIssues[i].Raw,
				Status:     storeIssues[i].Status,
				AssignedTo: storeIssues[i].AssignedTo,
				Tags:       item.TagScores,
				X:          position.X,
				Y:          position.Y,
				Hubness:    issueanalytics.MapProjectionIssueHubness(storeIssues[i]),
			}
			mapIssues[i] = mapIssue
		}
		profile.Steps = append(profile.Steps, BuildMapProjectionStep{
			Name:     "materialize_map_issues",
			Duration: time.Since(stepStartedAt),
		})

		stepStartedAt = time.Now()
		allEdges = ComputeEdgesWithEmbeddings(runtimeIssues, issueEmbeddings, 0)
		profile.EdgeCount = len(allEdges)
		profile.Steps = append(profile.Steps, BuildMapProjectionStep{
			Name:     "compute_edges",
			Duration: time.Since(stepStartedAt),
		})

		stepStartedAt = time.Now()
		sortEdgesBySimilarity(allEdges)
		profile.Steps = append(profile.Steps, BuildMapProjectionStep{
			Name:     "sort_edges",
			Duration: time.Since(stepStartedAt),
		})

		stepStartedAt = time.Now()
		clusters = ComputeFactorClusters(runtimeIssues, positions)
		profile.ClusterCount = len(clusters)
		profile.Steps = append(profile.Steps, BuildMapProjectionStep{
			Name:     "compute_clusters",
			Duration: time.Since(stepStartedAt),
		})
	}

	profile.TotalDuration = time.Since(startedAt)

	return MapProjection{
		Available:         true,
		IssueCount:        len(runtimeIssues),
		MinimumIssueCount: minMapIssueCount,
		MapIssues:         append([]MapIssue(nil), mapIssues...),
		AllEdges:          append([]Edge(nil), allEdges...),
		Clusters:          cloneClusters(clusters),
		VisibleIssueIDs:   cloneVisibleIDs(visible),
	}, profile, nil
}

func BuildMapFromProjection(projection MapProjection, viewport *Viewport, edgeThreshold float64) (MapResponse, error) {
	if projection.UnavailableReason != "" {
		return unavailableMapResponse(projection.UnavailableReason, projection.IssueCount), nil
	}
	if len(projection.MapIssues) < minMapIssueCount {
		return unavailableMapResponse(mapUnavailableReasonInsufficientIssueCount, len(projection.MapIssues)), nil
	}

	visibleMapIssues := filterVisibleMapIssues(projection.MapIssues, projection.VisibleIssueIDs)
	base := mapBaseData{
		mapIssues:      visibleMapIssues,
		positions:      positionsFromMapIssues(visibleMapIssues),
		candidateEdges: filterEdgesByThresholdAndVisibility(projection.AllEdges, edgeThreshold, projection.VisibleIssueIDs),
		clusters:       filterVisibleClusters(projection.Clusters, projection.VisibleIssueIDs),
	}

	edges := filterEdgesForViewport(base, normalizeViewport(viewport))
	if edges == nil {
		edges = []Edge{}
	}
	if base.clusters == nil {
		base.clusters = []Cluster{}
	}

	return MapResponse{
		Available:         true,
		IssueCount:        len(projection.MapIssues),
		MinimumIssueCount: minMapIssueCount,
		Issues:            base.mapIssues,
		Edges:             edges,
		Clusters:          base.clusters,
	}, nil
}

func mapProjectionUnavailableReason(issueCount, tagCount int) string {
	if issueCount < minMapIssueCount {
		return mapUnavailableReasonInsufficientIssueCount
	}
	if tagCount < 2 {
		return mapUnavailableReasonInsufficientDimensions
	}
	return ""
}

func BuildEdgeResponseFromProjection(projection MapProjection, viewport *Viewport, edgeThreshold float64) (EdgeResponse, error) {
	if projection.UnavailableReason != "" || len(projection.MapIssues) < minMapIssueCount {
		return EdgeResponse{Edges: []Edge{}}, nil
	}

	visibleMapIssues := filterVisibleMapIssues(projection.MapIssues, projection.VisibleIssueIDs)
	base := mapBaseData{
		mapIssues:      visibleMapIssues,
		positions:      positionsFromMapIssues(visibleMapIssues),
		candidateEdges: filterEdgesByThresholdAndVisibility(projection.AllEdges, edgeThreshold, projection.VisibleIssueIDs),
		clusters:       filterVisibleClusters(projection.Clusters, projection.VisibleIssueIDs),
	}

	edges := filterEdgesForViewport(base, normalizeViewport(viewport))
	if edges == nil {
		edges = []Edge{}
	}
	return EdgeResponse{Edges: edges}, nil
}

type relationshipIssue struct {
	ID    string
	Links []issues.IssueLink
}

func deriveRelationshipSemantics(items []issues.Issue) (map[string]string, map[string]struct{}, map[string]map[string]float64) {
	relationshipItems := make([]relationshipIssue, len(items))
	for i, item := range items {
		relationshipItems[i] = relationshipIssue{ID: item.ID, Links: item.Links}
	}
	return deriveRelationshipSemanticsFromItems(relationshipItems)
}

func deriveProjectionRelationshipSemantics(
	items []issues.MapProjectionIssue,
) (map[string]string, map[string]struct{}, map[string]map[string]float64) {
	relationshipItems := make([]relationshipIssue, len(items))
	for i, item := range items {
		relationshipItems[i] = relationshipIssue{ID: item.ID, Links: item.Links}
	}
	return deriveRelationshipSemanticsFromItems(relationshipItems)
}

func deriveRelationshipSemanticsFromItems(
	items []relationshipIssue,
) (map[string]string, map[string]struct{}, map[string]map[string]float64) {
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

func filterMapIssuesByID(items []MapIssue, ids map[string]struct{}) []MapIssue {
	filtered := make([]MapIssue, 0, len(ids))
	for _, item := range items {
		if _, ok := ids[item.ID]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func positionsFromMapIssues(items []MapIssue) map[string]Position {
	if len(items) == 0 {
		return map[string]Position{}
	}

	positions := make(map[string]Position, len(items))
	for _, item := range items {
		positions[item.ID] = Position{X: item.X, Y: item.Y}
	}
	return positions
}

func filterEdgesByID(items []Edge, ids map[string]struct{}) []Edge {
	filtered := make([]Edge, 0, len(items))
	for _, item := range items {
		if _, ok := ids[item.Source]; !ok {
			continue
		}
		if _, ok := ids[item.Target]; !ok {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func cloneVisibleIDs(input map[string]struct{}) map[string]struct{} {
	if len(input) == 0 {
		return map[string]struct{}{}
	}

	out := make(map[string]struct{}, len(input))
	for id := range input {
		out[id] = struct{}{}
	}
	return out
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
