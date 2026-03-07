package issuemap

import (
	"math"
	"sort"
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
	if len(issues) < 2 || threshold <= 0 {
		return nil
	}

	groups := groupIssuesByProximity(issues, positions, threshold)

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

type cellKey struct {
	x int
	y int
}

type disjointSet struct {
	parent []int
	rank   []uint8
}

func newDisjointSet(size int) *disjointSet {
	parent := make([]int, size)
	rank := make([]uint8, size)
	for i := range parent {
		parent[i] = i
	}
	return &disjointSet{parent: parent, rank: rank}
}

func (d *disjointSet) find(x int) int {
	root := x
	for d.parent[root] != root {
		root = d.parent[root]
	}
	for d.parent[x] != x {
		parent := d.parent[x]
		d.parent[x] = root
		x = parent
	}
	return root
}

func (d *disjointSet) union(a, b int) {
	rootA := d.find(a)
	rootB := d.find(b)
	if rootA == rootB {
		return
	}

	if d.rank[rootA] < d.rank[rootB] {
		rootA, rootB = rootB, rootA
	}
	d.parent[rootB] = rootA
	if d.rank[rootA] == d.rank[rootB] {
		d.rank[rootA]++
	}
}

func groupIssuesByProximity(issues []Issue, positions map[string]Position, threshold float64) [][]Issue {
	grid := make(map[cellKey][]int, len(issues))
	sets := newDisjointSet(len(issues))
	thresholdSq := threshold * threshold

	for i, issue := range issues {
		pos := positions[issue.ID]
		cell := positionCell(pos, threshold)

		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				neighbor := cellKey{x: cell.x + dx, y: cell.y + dy}
				for _, otherIdx := range grid[neighbor] {
					otherPos := positions[issues[otherIdx].ID]
					if squaredDistance(pos, otherPos) <= thresholdSq {
						sets.union(i, otherIdx)
					}
				}
			}
		}

		grid[cell] = append(grid[cell], i)
	}

	groupsByRoot := make(map[int][]Issue, len(issues))
	order := make([]int, 0, len(issues))
	for i, issue := range issues {
		root := sets.find(i)
		if _, ok := groupsByRoot[root]; !ok {
			order = append(order, root)
		}
		groupsByRoot[root] = append(groupsByRoot[root], issue)
	}

	groups := make([][]Issue, 0, len(order))
	for _, root := range order {
		group := groupsByRoot[root]
		if len(group) >= 2 {
			groups = append(groups, group)
		}
	}
	return groups
}

func positionCell(pos Position, cellSize float64) cellKey {
	return cellKey{
		x: int(math.Floor(pos.X / cellSize)),
		y: int(math.Floor(pos.Y / cellSize)),
	}
}

func squaredDistance(a, b Position) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return dx*dx + dy*dy
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
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].v == sorted[j].v {
			return sorted[i].k < sorted[j].k
		}
		return sorted[i].v > sorted[j].v
	})

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
