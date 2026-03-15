package vectors

import (
	"math"
	"math/rand/v2"
)

const (
	kmeansMaxIterations        = 50
	kmeansConvergenceThreshold = 1e-6
	kmeansMinK                 = 2
	specificityOutlierCap      = 0.8
)

// KMeansResult holds the per-point cluster assignment and embedding specificity score.
type KMeansResult struct {
	Cluster     int
	Distance    float64 // cosine distance from centroid
	Specificity float64 // normalized 0–1 within cluster, capped at outlierCap
}

// KMeansSpecificity runs k-means clustering on the given embeddings and returns
// a per-embedding specificity score based on cosine distance from the assigned
// cluster centroid. k = floor(sqrt(n)), minimum 2.
//
// Each embedding's distance from its centroid is normalized to [0, 1] within the
// cluster (max distance in cluster = 1.0), then capped at the outlier cap (0.8)
// to avoid pathological cases.
func KMeansSpecificity(embeddings [][]float64) []KMeansResult {
	n := len(embeddings)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return []KMeansResult{{Cluster: 0, Distance: 0, Specificity: 0}}
	}

	k := int(math.Floor(math.Sqrt(float64(n))))
	if k < kmeansMinK {
		k = kmeansMinK
	}
	if k > n {
		k = n
	}

	centroids := initCentroids(embeddings, k)
	assignments := make([]int, n)

	for iter := range kmeansMaxIterations {
		_ = iter
		changed := false
		for i, emb := range embeddings {
			best := nearestCentroid(emb, centroids)
			if best != assignments[i] {
				assignments[i] = best
				changed = true
			}
		}

		if !changed {
			break
		}

		newCentroids := updateCentroids(embeddings, assignments, k)
		maxShift := 0.0
		for j := range centroids {
			shift := cosineDistance(centroids[j], newCentroids[j])
			if shift > maxShift {
				maxShift = shift
			}
		}
		centroids = newCentroids
		if maxShift < kmeansConvergenceThreshold {
			break
		}
	}

	// Compute cosine distances from centroids.
	distances := make([]float64, n)
	for i, emb := range embeddings {
		distances[i] = cosineDistance(emb, centroids[assignments[i]])
	}

	// Normalize per cluster and cap.
	results := make([]KMeansResult, n)
	clusterMaxDist := make(map[int]float64, k)
	for i, dist := range distances {
		c := assignments[i]
		if dist > clusterMaxDist[c] {
			clusterMaxDist[c] = dist
		}
		results[i].Cluster = c
		results[i].Distance = dist
	}
	for i := range results {
		c := results[i].Cluster
		maxDist := clusterMaxDist[c]
		if maxDist > 0 {
			results[i].Specificity = results[i].Distance / maxDist
		}
		if results[i].Specificity > specificityOutlierCap {
			results[i].Specificity = specificityOutlierCap
		}
	}

	return results
}

func initCentroids(embeddings [][]float64, k int) [][]float64 {
	n := len(embeddings)
	perm := rand.Perm(n)
	centroids := make([][]float64, k)
	for i := range k {
		centroids[i] = copyVec(embeddings[perm[i]])
	}
	return centroids
}

func nearestCentroid(point []float64, centroids [][]float64) int {
	best := 0
	bestDist := math.Inf(1)
	for j, c := range centroids {
		d := cosineDistance(point, c)
		if d < bestDist {
			bestDist = d
			best = j
		}
	}
	return best
}

func updateCentroids(embeddings [][]float64, assignments []int, k int) [][]float64 {
	if len(embeddings) == 0 {
		return make([][]float64, k)
	}
	dim := len(embeddings[0])
	sums := make([][]float64, k)
	counts := make([]int, k)
	for i := range k {
		sums[i] = make([]float64, dim)
	}

	for i, emb := range embeddings {
		c := assignments[i]
		counts[c]++
		for d := range emb {
			sums[c][d] += emb[d]
		}
	}

	centroids := make([][]float64, k)
	for j := range k {
		centroids[j] = make([]float64, dim)
		if counts[j] == 0 {
			continue
		}
		for d := range dim {
			centroids[j][d] = sums[j][d] / float64(counts[j])
		}
		normalize(centroids[j])
	}
	return centroids
}

func cosineDistance(a, b []float64) float64 {
	sim := CosineSimilarity(a, b)
	return 1 - sim
}

func normalize(v []float64) {
	var mag float64
	for _, val := range v {
		mag += val * val
	}
	mag = math.Sqrt(mag)
	if mag == 0 {
		return
	}
	for i := range v {
		v[i] /= mag
	}
}

func copyVec(v []float64) []float64 {
	out := make([]float64, len(v))
	copy(out, v)
	return out
}
