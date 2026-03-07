package issuemap

import (
	"fmt"
	"math"
	"sort"
	"sync"
)

const (
	minEdgeSimilarity   = 0.85
	minVisibleEdgeCount = 24
	maxVisibleEdgeRatio = 0.2
	maxVisibleEdges     = 180
)

type MapIssue struct {
	ID   string         `json:"id"`
	Raw  string         `json:"raw"`
	Tags []TagRelevance `json:"tags"`
	X    float64        `json:"x"`
	Y    float64        `json:"y"`
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
	issues := AllIssues()
	tags := AllTags()
	tagEmbeddings := AllTagEmbeddings()

	positions, err := ComputePositions(issues, tags, tagEmbeddings)
	if err != nil {
		return mapBaseData{}, err
	}

	mapIssues := make([]MapIssue, len(issues))
	for i, issue := range issues {
		p, ok := positions[issue.ID]
		if !ok {
			return mapBaseData{}, fmt.Errorf("missing position for issue %s", issue.ID)
		}

		mapIssues[i] = MapIssue{
			ID:   issue.ID,
			Raw:  issue.Raw,
			Tags: issue.Tags,
			X:    math.Round(p.X*1000) / 1000,
			Y:    math.Round(p.Y*1000) / 1000,
		}
	}

	edges := ComputeEdges(issues, minEdgeSimilarity)
	clusters := ComputeClusters(issues, positions, 0.18)

	return mapBaseData{
		mapIssues:      mapIssues,
		positions:      positions,
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

	if v.XMin == v.XMax || v.YMin == v.YMax {
		return Viewport{XMin: 0, XMax: 1, YMin: 0, YMax: 1}
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

	if len(visibleIDs) < 2 {
		return []Edge{}
	}

	candidates := make([]Edge, 0, len(base.candidateEdges))
	for _, edge := range base.candidateEdges {
		_, sourceVisible := visibleIDs[edge.Source]
		_, targetVisible := visibleIDs[edge.Target]
		if !sourceVisible && !targetVisible {
			continue
		}
		candidates = append(candidates, edge)
	}

	targetEdgeCount := maxInt(
		minVisibleEdgeCount,
		int(math.Ceil(float64(len(visibleIDs))*maxVisibleEdgeRatio)),
	)
	targetEdgeCount = minInt(targetEdgeCount, maxVisibleEdges)
	if len(candidates) <= targetEdgeCount {
		return candidates
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Similarity == candidates[j].Similarity {
			if candidates[i].Source == candidates[j].Source {
				return candidates[i].Target < candidates[j].Target
			}
			return candidates[i].Source < candidates[j].Source
		}
		return candidates[i].Similarity > candidates[j].Similarity
	})

	return candidates[:targetEdgeCount]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
