package issuemap

import (
	"cmp"
	"math"
	"slices"

	"splat/internal/issues"
)

const (
	minEdgeSimilarity   = 0.40
	minVisibleEdgeCount = 24
	maxVisibleEdgeRatio = 0.2
	maxVisibleEdges     = 180
	minMapIssueCount    = 5
)

type MapIssue struct {
	ID         string             `json:"id"`
	Raw        string             `json:"raw"`
	Status     issues.IssueStatus `json:"status"`
	AssignedTo string             `json:"assignedTo,omitempty"`
	Tags       []TagRelevance     `json:"tags"`
	X          float64            `json:"x"`
	Y          float64            `json:"y"`
	Hubness    float64            `json:"hubness"`
}

type MapResponse struct {
	Available         bool       `json:"available"`
	UnavailableReason string     `json:"unavailableReason,omitempty"`
	IssueCount        int        `json:"issueCount"`
	MinimumIssueCount int        `json:"minimumIssueCount"`
	Issues            []MapIssue `json:"issues"`
	Edges             []Edge     `json:"edges"`
	Clusters          []Cluster  `json:"clusters"`
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

const (
	mapUnavailableReasonInsufficientIssueCount = "insufficient_issue_count"
	mapUnavailableReasonInsufficientDimensions = "insufficient_projection_dimensions"
)

func unavailableMapResponse(reason string, issueCount int) MapResponse {
	return MapResponse{
		Available:         false,
		UnavailableReason: reason,
		IssueCount:        issueCount,
		MinimumIssueCount: minMapIssueCount,
		Issues:            []MapIssue{},
		Edges:             []Edge{},
		Clusters:          []Cluster{},
	}
}

func UnavailableMapResponse(reason string, issueCount int) MapResponse {
	return unavailableMapResponse(reason, issueCount)
}

func MinimumMapIssueCount() int {
	return minMapIssueCount
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
	targetEdgeCount = min(targetEdgeCount, maxVisibleEdges)

	bothVisible := make([]Edge, 0, min(targetEdgeCount, len(base.candidateEdges)))
	oneVisible := make([]Edge, 0, min(targetEdgeCount, len(base.candidateEdges)))
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

	candidates := make([]Edge, 0, min(targetEdgeCount, len(bothVisible)+len(oneVisible)))
	candidates = append(candidates, bothVisible...)
	if len(candidates) >= targetEdgeCount {
		return candidates[:targetEdgeCount]
	}

	remaining := min(targetEdgeCount-len(candidates), len(oneVisible))
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
	slices.SortStableFunc(edges, func(a, b Edge) int {
		if c := cmp.Compare(b.Similarity, a.Similarity); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Source, b.Source); c != 0 {
			return c
		}
		return cmp.Compare(a.Target, b.Target)
	})
}
