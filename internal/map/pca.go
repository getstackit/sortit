package issuemap

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/mat"
)

type Position struct {
	X float64
	Y float64
}

func ComputePositions(issues []Issue, tags []string, tagEmbeddings map[string][]float64) (map[string]Position, error) {
	n := len(issues)
	t := len(tags)
	if n == 0 {
		return map[string]Position{}, nil
	}
	if n < 2 || t < 2 {
		return fallbackPositions(issues), nil
	}

	tagIndex := make(map[string]int, t)
	for i, tag := range tags {
		tagIndex[tag] = i
	}

	// Build N×T relevance matrix
	data := make([]float64, n*t)
	for i, issue := range issues {
		for _, tr := range issue.Tags {
			if j, ok := tagIndex[tr.Tag]; ok {
				data[i*t+j] = tr.Relevance
			}
		}
	}
	X := mat.NewDense(n, t, data)

	// Build T×T tag covariance matrix from tag embeddings (cosine similarity)
	tagCov := buildTagCovariance(tags, tagEmbeddings)

	// Transform: X' = X × Σ_tags — smears loadings across correlated tags
	var Xprime mat.Dense
	Xprime.Mul(X, tagCov)

	// Mean-center each column
	for j := 0; j < t; j++ {
		col := mat.Col(nil, j, &Xprime)
		mean := 0.0
		for _, v := range col {
			mean += v
		}
		mean /= float64(n)
		for i := 0; i < n; i++ {
			Xprime.Set(i, j, Xprime.At(i, j)-mean)
		}
	}

	// Covariance: C = (1/(N-1)) * X'ᵀ * X'
	var covRaw mat.Dense
	covRaw.Mul(Xprime.T(), &Xprime)
	covRaw.Scale(1.0/float64(n-1), &covRaw)

	// Convert to SymDense for EigenSym
	covSym := mat.NewSymDense(t, nil)
	for i := 0; i < t; i++ {
		for j := i; j < t; j++ {
			covSym.SetSym(i, j, covRaw.At(i, j))
		}
	}

	// Eigendecompose
	var eig mat.EigenSym
	ok := eig.Factorize(covSym, true)
	if !ok {
		return nil, fmt.Errorf("factorize covariance matrix")
	}

	var eigVecs mat.Dense
	eig.VectorsTo(&eigVecs)

	// Top 2 eigenvectors (last 2 columns — gonum returns ascending order)
	v2Data := make([]float64, t*2)
	for i := 0; i < t; i++ {
		v2Data[i*2] = eigVecs.At(i, t-1)
		v2Data[i*2+1] = eigVecs.At(i, t-2)
	}
	V2 := mat.NewDense(t, 2, v2Data)

	// Project: P = X' * V2
	var P mat.Dense
	P.Mul(&Xprime, V2)

	// Min-max normalize to [0.05, 0.95]
	positions := make(map[string]Position, n)
	xs := make([]float64, n)
	ys := make([]float64, n)
	for i := 0; i < n; i++ {
		xs[i] = P.At(i, 0)
		ys[i] = P.At(i, 1)
	}

	normalize(xs, 0.05, 0.95)
	normalize(ys, 0.05, 0.95)

	// Sign convention: if first point has negative projected x before normalization, flip
	// We apply after normalize by checking if the order looks inverted
	if n > 1 && P.At(0, 0) < 0 {
		for i := range xs {
			xs[i] = 1.0 - xs[i]
		}
	}
	if n > 1 && P.At(0, 1) < 0 {
		for i := range ys {
			ys[i] = 1.0 - ys[i]
		}
	}

	for i, issue := range issues {
		positions[issue.ID] = Position{X: xs[i], Y: ys[i]}
	}

	return positions, nil
}

func normalize(vals []float64, lo, hi float64) {
	minV, maxV := vals[0], vals[0]
	for _, v := range vals[1:] {
		minV = math.Min(minV, v)
		maxV = math.Max(maxV, v)
	}
	span := maxV - minV
	if span == 0 {
		mid := (lo + hi) / 2
		for i := range vals {
			vals[i] = mid
		}
		return
	}
	for i, v := range vals {
		vals[i] = lo + (v-minV)/span*(hi-lo)
	}
}

// buildTagCovariance computes a T×T matrix where entry (i,j) is the cosine
// similarity between the embeddings of tag i and tag j. If embeddings are
// missing, falls back to the identity matrix (tags treated as independent).
func buildTagCovariance(tags []string, tagEmbeddings map[string][]float64) *mat.Dense {
	t := len(tags)
	data := make([]float64, t*t)

	hasEmbeddings := len(tagEmbeddings) > 0
	for i := 0; i < t; i++ {
		for j := 0; j < t; j++ {
			if !hasEmbeddings {
				if i == j {
					data[i*t+j] = 1
				}
				continue
			}
			data[i*t+j] = cosineSimilarity(tagEmbeddings[tags[i]], tagEmbeddings[tags[j]])
		}
	}

	return mat.NewDense(t, t, data)
}

func fallbackPositions(issues []Issue) map[string]Position {
	positions := make(map[string]Position, len(issues))
	if len(issues) == 1 {
		positions[issues[0].ID] = Position{X: 0.5, Y: 0.5}
		return positions
	}

	const radius = 0.35
	for i, issue := range issues {
		angle := (2 * math.Pi * float64(i)) / float64(len(issues))
		positions[issue.ID] = Position{
			X: 0.5 + radius*math.Cos(angle),
			Y: 0.5 + radius*math.Sin(angle),
		}
	}

	return positions
}
