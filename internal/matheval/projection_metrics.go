package matheval

import (
	"math"
	"slices"

	"sortit/internal/issuemath"
	"sortit/internal/vectors"
)

// This file holds the map-projection quality metrics (WP-303): pure functions
// over a 2-D layout (issuemath.ComputePositionsAligned output) plus reference
// data. They are the map's answer to the ranking harness's NDCG/Recall — three
// complementary, deliberately un-collapsed measurements so a successor
// projection (Stage 5 replaces the input entirely) can be argued against each
// individually rather than by one flattering aggregate. See docs/math-eval.md.

// --- Neighborhood construction ---

// layoutNeighbors returns, for each id, the k nearest OTHER ids by Euclidean
// distance in the 2-D layout, ties broken by id so the ordering is
// deterministic. The layout positions are in the [0.05, 0.95] normalized square
// ComputePositionsAligned emits.
func layoutNeighbors(positions map[string]issuemath.Position, ids []string, k int) map[string][]string {
	out := make(map[string][]string, len(ids))
	for _, id := range ids {
		p := positions[id]
		out[id] = nearestK(ids, id, k, func(other string) float64 {
			q := positions[other]
			return math.Hypot(p.X-q.X, p.Y-q.Y)
		})
	}
	return out
}

// embeddingNeighbors returns, for each id, the k nearest OTHER ids by cosine
// distance (1 − cosine) over the provided embeddings. The caller passes the
// corpus-mean-centered embeddings, so "close" means close in the same geometry
// the similarity math uses (whitepaper §2.0).
func embeddingNeighbors(embeddings map[string][]float64, ids []string, k int) map[string][]string {
	out := make(map[string][]string, len(ids))
	for _, id := range ids {
		v := embeddings[id]
		out[id] = nearestK(ids, id, k, func(other string) float64 {
			return 1 - vectors.UnitCosineSimilarity(v, embeddings[other])
		})
	}
	return out
}

// nearestK returns the k ids (excluding self) with the smallest distance,
// breaking ties by id so the result is deterministic across runs and platforms.
func nearestK(ids []string, self string, k int, dist func(other string) float64) []string {
	type neighbor struct {
		id string
		d  float64
	}
	cand := make([]neighbor, 0, len(ids))
	for _, id := range ids {
		if id == self {
			continue
		}
		cand = append(cand, neighbor{id: id, d: dist(id)})
	}
	slices.SortFunc(cand, func(a, b neighbor) int {
		if a.d != b.d {
			if a.d < b.d {
				return -1
			}
			return 1
		}
		if a.id < b.id {
			return -1
		}
		if a.id > b.id {
			return 1
		}
		return 0
	})
	if k > len(cand) {
		k = len(cand)
	}
	out := make([]string, k)
	for i := range k {
		out[i] = cand[i].id
	}
	return out
}

// --- Metric 1: neighborhood preservation ---

// meanKNNOverlap is the symmetric kNN-overlap neighborhood-preservation score:
// the mean over ids of |a[id] ∩ b[id]| / k, where a is each id's layout
// neighborhood and b its reference neighborhood. 1.0 means every item's
// layout neighbors are exactly its reference neighbors; the chance level of a
// random layout is ≈ k/(n−1).
func meanKNNOverlap(ids []string, a, b map[string][]string, k int) float64 {
	if len(ids) == 0 || k == 0 {
		return 0
	}
	sum := 0.0
	for _, id := range ids {
		set := make(map[string]struct{}, len(a[id]))
		for _, n := range a[id] {
			set[n] = struct{}{}
		}
		hits := 0
		for _, n := range b[id] {
			if _, ok := set[n]; ok {
				hits++
			}
		}
		sum += float64(hits) / float64(k)
	}
	return sum / float64(len(ids))
}

// meanDomainPurity is the ground-truth-region flavor of neighborhood
// preservation: the mean over ids of the fraction of each id's layout-kNN that
// share its primary ground-truth domain. Items with no primary domain (the
// untagged off-taxonomy notes) are skipped as query points but stay eligible as
// neighbors — a note landing in a neighborhood does not count for or against
// purity. 1.0 means every layout neighborhood is domain-pure.
func meanDomainPurity(ids []string, layoutNN map[string][]string, domainOf map[string]string) float64 {
	sum := 0.0
	counted := 0
	for _, id := range ids {
		dom := domainOf[id]
		if dom == "" {
			continue
		}
		nn := layoutNN[id]
		if len(nn) == 0 {
			continue
		}
		same := 0
		for _, n := range nn {
			if domainOf[n] == dom {
				same++
			}
		}
		sum += float64(same) / float64(len(nn))
		counted++
	}
	if counted == 0 {
		return 0
	}
	return sum / float64(counted)
}

// --- Metric 2: cluster legibility ---

// silhouetteByLabel computes the mean silhouette of a label assignment under
// 2-D Euclidean distance over the layout. It mirrors the k-means silhouette in
// internal/map/cluster.go (silhouetteScore), generalized from integer cluster
// indices to string ground-truth labels: for each item a is the mean distance
// to same-label items and b the min mean distance to any OTHER label, and the
// silhouette is (b−a)/max(a,b) ∈ [−1, 1]. Items whose label has a single member
// contribute 0 (cohesion undefined); items with an empty label are excluded.
// +1 means the layout separates the labels cleanly; ≤ 0 means the ground-truth
// regions are collapsed or interleaved in the layout.
func silhouetteByLabel(positions map[string]issuemath.Position, labels map[string]string) float64 {
	ids := make([]string, 0, len(labels))
	for id, lab := range labels {
		if lab == "" {
			continue
		}
		if _, ok := positions[id]; !ok {
			continue
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)

	counts := map[string]int{}
	for _, id := range ids {
		counts[labels[id]]++
	}

	dist := func(a, b string) float64 {
		p, q := positions[a], positions[b]
		return math.Hypot(p.X-q.X, p.Y-q.Y)
	}

	total := 0.0
	valid := 0
	for _, i := range ids {
		li := labels[i]
		if counts[li] <= 1 {
			continue
		}

		a := 0.0
		for _, j := range ids {
			if j != i && labels[j] == li {
				a += dist(i, j)
			}
		}
		a /= float64(counts[li] - 1)

		b := math.MaxFloat64
		for lab, c := range counts {
			if lab == li || c == 0 {
				continue
			}
			sum := 0.0
			for _, j := range ids {
				if labels[j] == lab {
					sum += dist(i, j)
				}
			}
			if mean := sum / float64(c); mean < b {
				b = mean
			}
		}

		if denom := math.Max(a, b); denom > 0 {
			total += (b - a) / denom
		}
		valid++
	}

	if valid == 0 {
		return 0
	}
	return total / float64(valid)
}

// --- Metric 3: refresh stability ---

// layoutDisplacements returns the per-issue displacement between two layouts,
// over the ids present in both. Distances are in the layouts' own units (the
// [0.05, 0.95] normalized square), so a value of 0.1 is a tenth of the map's
// span. Issues present in only one layout (added or removed by a mutation)
// carry no displacement and are omitted.
func layoutDisplacements(prev, cur map[string]issuemath.Position) []float64 {
	out := make([]float64, 0, len(cur))
	for id, c := range cur {
		p, ok := prev[id]
		if !ok {
			continue
		}
		out = append(out, math.Hypot(c.X-p.X, c.Y-p.Y))
	}
	return out
}

// meanAndP95 returns the mean and 95th-percentile of a sample (linear
// interpolation between order statistics, the same convention as Summarize).
// Used to reduce a pooled displacement sample to its baseline numbers.
func meanAndP95(values []float64) (mean, p95 float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sorted := append([]float64(nil), values...)
	slices.Sort(sorted)
	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	return sum / float64(len(sorted)), percentile(sorted, 0.95)
}
