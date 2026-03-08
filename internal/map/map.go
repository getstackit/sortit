package issuemap

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"splat/internal/issues"
)

const (
	minEdgeSimilarity   = 0.40
	minVisibleEdgeCount = 24
	maxVisibleEdgeRatio = 0.2
	maxVisibleEdges     = 180
)

type MapIssue struct {
	ID     string             `json:"id"`
	Raw    string             `json:"raw"`
	Status issues.IssueStatus `json:"status"`
	Tags   []TagRelevance     `json:"tags"`
	X      float64            `json:"x"`
	Y      float64            `json:"y"`
}

type MapResponse struct {
	Issues   []MapIssue `json:"issues"`
	Edges    []Edge     `json:"edges"`
	Clusters []Cluster  `json:"clusters"`
}

type EdgeResponse struct {
	Edges []Edge `json:"edges"`
}

type Viewport struct {
	XMin float64
	XMax float64
	YMin float64
	YMax float64
}

type mapBaseData struct {
	mapIssues      []MapIssue
	positions      map[string]Position
	candidateEdges []Edge
	clusters       []Cluster
}

var (
	baseMapOnce      sync.Once
	baseMapDataCache mapBaseData
	baseMapErr       error
)

func BuildMap(viewport *Viewport) (MapResponse, error) {
	base, err := loadBaseMapData()
	if err != nil {
		return MapResponse{}, err
	}

	edges := filterEdgesForViewport(base, normalizeViewport(viewport))
	if edges == nil {
		edges = []Edge{}
	}

	clusters := base.clusters
	if clusters == nil {
		clusters = []Cluster{}
	}

	return MapResponse{
		Issues:   base.mapIssues,
		Edges:    edges,
		Clusters: clusters,
	}, nil
}

func BuildEdgeResponse(viewport *Viewport) (EdgeResponse, error) {
	base, err := loadBaseMapData()
	if err != nil {
		return EdgeResponse{}, err
	}

	edges := filterEdgesForViewport(base, normalizeViewport(viewport))
	if edges == nil {
		edges = []Edge{}
	}

	return EdgeResponse{Edges: edges}, nil
}

func loadBaseMapData() (mapBaseData, error) {
	baseMapOnce.Do(func() {
		baseMapDataCache, baseMapErr = buildBaseMapData()
	})
	return baseMapDataCache, baseMapErr
}

func buildBaseMapData() (mapBaseData, error) {
	allIssues := AllIssues()
	tags := AllTags()
	tagEmbeddings := AllTagEmbeddings()

	positions, err := ComputePositions(allIssues, tags, tagEmbeddings)
	if err != nil {
		return mapBaseData{}, err
	}

	mapIssues := make([]MapIssue, len(allIssues))
	roundedPositions := make(map[string]Position, len(allIssues))
	for i, issue := range allIssues {
		p, ok := positions[issue.ID]
		if !ok {
			return mapBaseData{}, fmt.Errorf("missing position for issue %s", issue.ID)
		}

		rounded := roundPosition(p)
		roundedPositions[issue.ID] = rounded

		mapIssues[i] = MapIssue{
			ID:     issue.ID,
			Raw:    issue.Raw,
			Status: issues.StatusOpen,
			Tags:   issue.Tags,
			X:      rounded.X,
			Y:      rounded.Y,
		}
	}

	edges := ComputeEdges(allIssues, minEdgeSimilarity)
	sortEdgesBySimilarity(edges)
	clusters := ComputeClusters(allIssues, positions, 0.18)

	return mapBaseData{
		mapIssues:      mapIssues,
		positions:      roundedPositions,
		candidateEdges: edges,
		clusters:       clusters,
	}, nil
}

func normalizeViewport(viewport *Viewport) Viewport {
	if viewport == nil {
		return Viewport{XMin: 0, XMax: 1, YMin: 0, YMax: 1}
	}

	v := *viewport
	if v.XMin > v.XMax {
		v.XMin, v.XMax = v.XMax, v.XMin
	}
	if v.YMin > v.YMax {
		v.YMin, v.YMax = v.YMax, v.YMin
	}

	return v
}

func filterEdgesForViewport(base mapBaseData, viewport Viewport) []Edge {
	visibleIDs := make(map[string]struct{}, len(base.positions))
	for id, position := range base.positions {
		if position.X >= viewport.XMin &&
			position.X <= viewport.XMax &&
			position.Y >= viewport.YMin &&
			position.Y <= viewport.YMax {
			visibleIDs[id] = struct{}{}
		}
	}

	if len(visibleIDs) == 0 {
		return []Edge{}
	}

	targetEdgeCount := maxInt(
		minVisibleEdgeCount,
		int(math.Ceil(float64(len(visibleIDs))*maxVisibleEdgeRatio)),
	)
	targetEdgeCount = minInt(targetEdgeCount, maxVisibleEdges)

	bothVisible := make([]Edge, 0, minInt(targetEdgeCount, len(base.candidateEdges)))
	oneVisible := make([]Edge, 0, minInt(targetEdgeCount, len(base.candidateEdges)))
	for _, edge := range base.candidateEdges {
		_, sourceVisible := visibleIDs[edge.Source]
		_, targetVisible := visibleIDs[edge.Target]
		if !sourceVisible && !targetVisible {
			continue
		}

		if sourceVisible && targetVisible {
			bothVisible = append(bothVisible, edge)
			continue
		}

		oneVisible = append(oneVisible, edge)
	}

	candidates := make([]Edge, 0, minInt(targetEdgeCount, len(bothVisible)+len(oneVisible)))
	candidates = append(candidates, bothVisible...)
	if len(candidates) >= targetEdgeCount {
		return candidates[:targetEdgeCount]
	}

	remaining := targetEdgeCount - len(candidates)
	if remaining > len(oneVisible) {
		remaining = len(oneVisible)
	}
	candidates = append(candidates, oneVisible[:remaining]...)
	return candidates
}

func roundPosition(position Position) Position {
	return Position{
		X: roundCoordinate(position.X),
		Y: roundCoordinate(position.Y),
	}
}

func roundCoordinate(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sortEdgesBySimilarity(edges []Edge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Similarity == edges[j].Similarity {
			if edges[i].Source == edges[j].Source {
				return edges[i].Target < edges[j].Target
			}
			return edges[i].Source < edges[j].Source
		}
		return edges[i].Similarity > edges[j].Similarity
	})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
