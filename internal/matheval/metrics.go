package matheval

import (
	"math"
	"slices"
)

// NDCGAtK computes normalized discounted cumulative gain for a ranked list of
// issue IDs against graded judgments (grade 0 or absent = irrelevant). Gains
// are exponential (2^grade − 1), so a grade-3 result is worth roughly twice a
// grade-2 plus a grade-1. Returns 0 when the judgment set has no positive
// grades.
func NDCGAtK(ranked []string, grades map[string]int, k int) float64 {
	ideal := make([]int, 0, len(grades))
	for _, grade := range grades {
		if grade > 0 {
			ideal = append(ideal, grade)
		}
	}
	if len(ideal) == 0 {
		return 0
	}
	slices.SortFunc(ideal, func(a, b int) int { return b - a })

	dcg := 0.0
	for i, id := range ranked {
		if i >= k {
			break
		}
		dcg += gain(grades[id]) / discount(i)
	}
	idcg := 0.0
	for i, grade := range ideal {
		if i >= k {
			break
		}
		idcg += gain(grade) / discount(i)
	}
	return dcg / idcg
}

func gain(grade int) float64 {
	if grade <= 0 {
		return 0
	}
	return math.Pow(2, float64(grade)) - 1
}

func discount(position int) float64 {
	return math.Log2(float64(position) + 2)
}

// RecallAtK computes the share of relevant issues (grade > 0) that appear in
// the top k of the ranked list. Returns 0 when nothing is relevant.
func RecallAtK(ranked []string, grades map[string]int, k int) float64 {
	relevant := 0
	for _, grade := range grades {
		if grade > 0 {
			relevant++
		}
	}
	if relevant == 0 {
		return 0
	}
	hits := 0
	for i, id := range ranked {
		if i >= k {
			break
		}
		if grades[id] > 0 {
			hits++
		}
	}
	return float64(hits) / float64(relevant)
}

// Distribution summarizes a sample of values.
type Distribution struct {
	Count  int     `json:"count"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	P10    float64 `json:"p10"`
	P90    float64 `json:"p90"`
}

// Summarize computes mean, median, and the 10th/90th percentiles (linear
// interpolation between order statistics) of a sample.
func Summarize(values []float64) Distribution {
	if len(values) == 0 {
		return Distribution{}
	}
	sorted := append([]float64(nil), values...)
	slices.Sort(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	return Distribution{
		Count:  len(sorted),
		Mean:   sum / float64(len(sorted)),
		Median: percentile(sorted, 0.5),
		P10:    percentile(sorted, 0.1),
		P90:    percentile(sorted, 0.9),
	}
}

// percentile returns the p-quantile of an ascending-sorted sample using
// linear interpolation (the same convention as numpy's default).
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	frac := pos - float64(lo)
	if lo+1 >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	return sorted[lo]*(1-frac) + sorted[lo+1]*frac
}
