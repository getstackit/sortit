package issuemath

import (
	"cmp"
	"math"
	"slices"

	"sortit/internal/issues"
	"sortit/internal/scoring"
)

func MeanTagProfile(matched []issues.PeopleAnalyticsIssue, tagSpecificity map[string]*float64) []issues.TagRelevance {
	if len(matched) == 0 {
		return []issues.TagRelevance{}
	}

	sums := make(map[string]float64)
	for _, issue := range matched {
		for _, ts := range issue.TagScores {
			weighted := ts.Relevance * specificityWeight(tagSpecificity[ts.Tag])
			sums[ts.Tag] += weighted
		}
	}

	profile := make([]issues.TagRelevance, 0, len(sums))
	for tag, sum := range sums {
		profile = append(profile, issues.TagRelevance{
			Tag:       tag,
			Relevance: roundTo2(sum / float64(len(matched))),
		})
	}

	slices.SortStableFunc(profile, func(a, b issues.TagRelevance) int {
		if c := cmp.Compare(b.Relevance, a.Relevance); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})

	return profile
}

func MeanEmbedding(matched []issues.PeopleAnalyticsIssue) []float64 {
	if len(matched) == 0 {
		return nil
	}

	var dim int
	for _, issue := range matched {
		if len(issue.Embedding) > 0 {
			dim = len(issue.Embedding)
			break
		}
	}
	if dim == 0 {
		return nil
	}

	mean := make([]float64, dim)
	count := 0
	for _, issue := range matched {
		if len(issue.Embedding) != dim {
			continue
		}
		for k, v := range issue.Embedding {
			mean[k] += v
		}
		count++
	}

	if count == 0 {
		return nil
	}

	var mag float64
	for k := range mean {
		mean[k] /= float64(count)
		mag += mean[k] * mean[k]
	}

	if mag > 0 {
		mag = math.Sqrt(mag)
		for k := range mean {
			mean[k] /= mag
		}
	}

	return mean
}

func TagProfileSimilarity(a, b []issues.TagRelevance) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	tags := make(map[string]struct{})
	aMap := make(map[string]float64, len(a))
	bMap := make(map[string]float64, len(b))
	for _, tr := range a {
		aMap[tr.Tag] = tr.Relevance
		tags[tr.Tag] = struct{}{}
	}
	for _, tr := range b {
		bMap[tr.Tag] = tr.Relevance
		tags[tr.Tag] = struct{}{}
	}

	var dot, magA, magB float64
	for tag := range tags {
		av, bv := aMap[tag], bMap[tag]
		dot += av * bv
		magA += av * av
		magB += bv * bv
	}

	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

func specificityWeight(s *float64) float64 {
	if s == nil {
		return scoring.GenericTagThreshold
	}
	return *s
}
