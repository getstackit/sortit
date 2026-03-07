package issuemap

import (
	"math"
	"strings"
)

type Cluster struct {
	Label   string  `json:"label"`
	CenterX float64 `json:"centerX"`
	CenterY float64 `json:"centerY"`
	Radius  float64 `json:"radius"`
	Color   string  `json:"color"`
}

var tagColors = map[string]string{
	"bug":         "#ef4444",
	"crash":       "#dc2626",
	"feature":     "#a855f7",
	"idea":        "#a855f7",
	"improvement": "#22c55e",
	"ui":          "#3b82f6",
	"ux":          "#3b82f6",
	"frontend":    "#60a5fa",
	"performance": "#f59e0b",
	"safari":      "#f59e0b",
	"onboarding":  "#06b6d4",
	"search":      "#8b5cf6",
	"export":      "#ec4899",
}

func ComputeClusters(issues []Issue, positions map[string]Position, threshold float64) []Cluster {
	visited := make(map[string]bool)
	var groups [][]Issue

	for _, issue := range issues {
		if visited[issue.ID] {
			continue
		}
		var group []Issue
		queue := []Issue{issue}
		for len(queue) > 0 {
			current := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			if visited[current.ID] {
				continue
			}
			visited[current.ID] = true
			group = append(group, current)
			cp := positions[current.ID]
			for _, other := range issues {
				if visited[other.ID] {
					continue
				}
				op := positions[other.ID]
				if math.Hypot(cp.X-op.X, cp.Y-op.Y) <= threshold {
					queue = append(queue, other)
				}
			}
		}
		if len(group) >= 2 {
			groups = append(groups, group)
		}
	}

	clusters := make([]Cluster, 0, len(groups))
	for _, group := range groups {
		cx, cy := 0.0, 0.0
		for _, issue := range group {
			p := positions[issue.ID]
			cx += p.X
			cy += p.Y
		}
		cx /= float64(len(group))
		cy /= float64(len(group))

		maxDist := 0.0
		for _, issue := range group {
			p := positions[issue.ID]
			d := math.Hypot(p.X-cx, p.Y-cy)
			if d > maxDist {
				maxDist = d
			}
		}

		label := clusterLabel(group)
		color := clusterColor(group)

		clusters = append(clusters, Cluster{
			Label:   label,
			CenterX: math.Round(cx*1000) / 1000,
			CenterY: math.Round(cy*1000) / 1000,
			Radius:  math.Round(maxDist*1000) / 1000,
			Color:   color,
		})
	}

	return clusters
}

func clusterLabel(group []Issue) string {
	scores := map[string]float64{}
	for _, issue := range group {
		for _, t := range issue.Tags {
			scores[t.Tag] += t.Relevance
		}
	}

	type kv struct {
		k string
		v float64
	}
	var sorted []kv
	for k, v := range scores {
		sorted = append(sorted, kv{k, v})
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].v > sorted[i].v {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	top := sorted
	if len(top) > 2 {
		top = top[:2]
	}

	parts := make([]string, len(top))
	for i, t := range top {
		parts[i] = strings.ToUpper(t.k[:1]) + t.k[1:]
	}
	return strings.Join(parts, " / ")
}

func clusterColor(group []Issue) string {
	scores := map[string]float64{}
	for _, issue := range group {
		for _, t := range issue.Tags {
			scores[t.Tag] += t.Relevance
		}
	}

	topTag := ""
	topScore := 0.0
	for tag, score := range scores {
		if score > topScore {
			topScore = score
			topTag = tag
		}
	}

	if c, ok := tagColors[topTag]; ok {
		return c
	}
	return "#94a3b8"
}
