package issuemap

import (
	"math"
	"sort"

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
