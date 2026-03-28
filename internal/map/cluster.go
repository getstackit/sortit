package issuemap

import (
	"cmp"
	"math"
	"math/rand"
	"slices"
	"strings"

	"splat/internal/issueanalytics"
	"splat/internal/issues"
)

type Cluster struct {
	Label    string   `json:"label"`
	CenterX  float64  `json:"centerX"`
	CenterY  float64  `json:"centerY"`
	Radius   float64  `json:"radius"`
	Color    string   `json:"color"`
	IssueIDs []string `json:"issueIds"`
	TopTag   string   `json:"topTag"`
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

// ComputeFactorClusters uses k-means clustering on 2D PCA positions.
// It tries k=3,4,5 and picks the best k by silhouette score.
// Clusters with fewer than 5 items are dissolved.
func ComputeFactorClusters(items []issues.Issue, positions map[string]Position) []Cluster {
	if len(items) < 5 {
		return nil
	}

	// Build ordered point list
	n := len(items)
	xs := make([]float64, n)
	ys := make([]float64, n)
	for i, issue := range items {
		p := positions[issue.ID]
		xs[i] = p.X
		ys[i] = p.Y
	}

	// Deterministic seed based on issue count
	rng := rand.New(rand.NewSource(int64(n))) //nolint:gosec

	bestScore := -2.0
	var bestAssign []int
	var bestCentroids [][2]float64

	for _, k := range []int{3, 4, 5} {
		if k > n {
			continue
		}
		assign, centroids := runKMeans(xs, ys, k, 50, rng)
		score := silhouetteScore(xs, ys, assign, k)
		if score > bestScore {
			bestScore = score
			bestAssign = assign
			bestCentroids = centroids
		}
	}

	if bestAssign == nil {
		return nil
	}

	// Group issues by cluster assignment
	groups := make(map[int][]issues.Issue)
	for i, issue := range items {
		c := bestAssign[i]
		groups[c] = append(groups[c], issue)
	}

	// Build cluster results, dissolving clusters smaller than 5
	var clusters []Cluster
	for c, group := range groups {
		if len(group) < 5 {
			continue
		}

		cx := bestCentroids[c][0]
		cy := bestCentroids[c][1]

		maxDist := 0.0
		ids := make([]string, len(group))
		for i, issue := range group {
			ids[i] = issue.ID
			p := positions[issue.ID]
			d := math.Hypot(p.X-cx, p.Y-cy)
			if d > maxDist {
				maxDist = d
			}
		}

		label := clusterLabel(group)
		color := clusterColor(group)
		topTag := clusterTopTag(group)

		clusters = append(clusters, Cluster{
			Label:    label,
			CenterX:  math.Round(cx*1000) / 1000,
			CenterY:  math.Round(cy*1000) / 1000,
			Radius:   math.Round(maxDist*1000) / 1000,
			Color:    color,
			IssueIDs: ids,
			TopTag:   topTag,
		})
	}

	return clusters
}

// runKMeans performs k-means clustering and returns assignments and final centroids.
func runKMeans(xs, ys []float64, k, maxIter int, rng *rand.Rand) ([]int, [][2]float64) {
	n := len(xs)

	// Random initialization: pick k distinct random indices as initial centroids
	perm := rng.Perm(n)
	centroids := make([][2]float64, k)
	for i := range k {
		centroids[i] = [2]float64{xs[perm[i]], ys[perm[i]]}
	}

	assign := make([]int, n)

	for iter := range maxIter {
		changed := false

		// Assignment step
		for i := range n {
			bestC := 0
			bestDist := math.MaxFloat64
			for c := range k {
				dx := xs[i] - centroids[c][0]
				dy := ys[i] - centroids[c][1]
				d := dx*dx + dy*dy
				if d < bestDist {
					bestDist = d
					bestC = c
				}
			}
			if assign[i] != bestC {
				assign[i] = bestC
				changed = true
			}
		}

		if !changed && iter > 0 {
			break
		}

		// Update step
		for c := range k {
			sumX, sumY := 0.0, 0.0
			count := 0
			for i := range n {
				if assign[i] == c {
					sumX += xs[i]
					sumY += ys[i]
					count++
				}
			}
			if count > 0 {
				centroids[c] = [2]float64{sumX / float64(count), sumY / float64(count)}
			}
		}
	}

	return assign, centroids
}

// silhouetteScore computes the average silhouette score for the clustering.
func silhouetteScore(xs, ys []float64, assign []int, k int) float64 {
	n := len(xs)
	if n < 2 || k < 2 {
		return -1
	}

	// Count members per cluster
	counts := make([]int, k)
	for _, c := range assign {
		counts[c]++
	}

	totalSil := 0.0
	validCount := 0

	for i := range n {
		ci := assign[i]
		if counts[ci] <= 1 {
			// Singleton cluster: silhouette is 0
			continue
		}

		// a = mean distance to same-cluster points
		a := 0.0
		for j := range n {
			if j != i && assign[j] == ci {
				a += math.Hypot(xs[i]-xs[j], ys[i]-ys[j])
			}
		}
		a /= float64(counts[ci] - 1)

		// b = min mean distance to other-cluster points
		b := math.MaxFloat64
		for c := range k {
			if c == ci || counts[c] == 0 {
				continue
			}
			sum := 0.0
			for j := range n {
				if assign[j] == c {
					sum += math.Hypot(xs[i]-xs[j], ys[i]-ys[j])
				}
			}
			mean := sum / float64(counts[c])
			if mean < b {
				b = mean
			}
		}

		denom := math.Max(a, b)
		if denom > 0 {
			totalSil += (b - a) / denom
		}
		validCount++
	}

	if validCount == 0 {
		return -1
	}
	return totalSil / float64(validCount)
}

// clusterTopTag returns the single top tag name for a group of issues.
// Hub issues (those with more links) contribute more heavily to the tag score,
// since they better represent cluster themes.
func clusterTopTag(group []issues.Issue) string {
	scores := map[string]float64{}
	for _, issue := range group {
		weight := 1 + issueanalytics.IssueHubness(issue)
		for _, t := range issue.TagScores {
			scores[t.Tag] += t.Relevance * weight
		}
	}

	topTag := ""
	topScore := 0.0
	for tag, score := range scores {
		if score > topScore || (score == topScore && tag < topTag) {
			topScore = score
			topTag = tag
		}
	}
	return topTag
}

func clusterLabel(group []issues.Issue) string {
	scores := map[string]float64{}
	for _, issue := range group {
		weight := 1 + issueanalytics.IssueHubness(issue)
		for _, t := range issue.TagScores {
			scores[t.Tag] += t.Relevance * weight
		}
	}

	type kv struct {
		k string
		v float64
	}
	sorted := make([]kv, 0, len(scores))
	for k, v := range scores {
		sorted = append(sorted, kv{k, v})
	}
	slices.SortStableFunc(sorted, func(a, b kv) int {
		if c := cmp.Compare(b.v, a.v); c != 0 {
			return c
		}
		return cmp.Compare(a.k, b.k)
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

func clusterColor(group []issues.Issue) string {
	scores := map[string]float64{}
	for _, issue := range group {
		weight := 1 + issueanalytics.IssueHubness(issue)
		for _, t := range issue.TagScores {
			scores[t.Tag] += t.Relevance * weight
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
