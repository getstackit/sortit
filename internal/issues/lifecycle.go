package issues

import (
	"cmp"
	"math"
	"slices"

	"sortit/internal/vectors"
)

const lifecycleRecencyDecay = 0.25

// ComputeIssueLifecycleMetrics derives churn from persisted canonical
// snapshots. Higher churn means an issue's meaning is still moving;
// callers needing "stability" derive it inline as (1 - churn).
func ComputeIssueLifecycleMetrics(snapshots []IssueSnapshot) *IssueLifecycleMetrics {
	if len(snapshots) < 2 {
		return nil
	}

	items := cloneIssueSnapshots(snapshots)
	slices.SortStableFunc(items, func(a, b IssueSnapshot) int {
		return cmp.Compare(a.Sequence, b.Sequence)
	})

	var (
		weightedTotal float64
		weightSum     float64
		transitions   int
	)

	for i := 1; i < len(items); i++ {
		delta, ok := issueSnapshotDelta(items[i-1], items[i])
		if !ok {
			continue
		}
		weight := math.Pow(lifecycleRecencyDecay, float64(len(items)-1-i))
		weightedTotal += delta * weight
		weightSum += weight
		transitions++
	}

	if transitions == 0 || weightSum == 0 {
		return nil
	}

	churn := clamp01(weightedTotal / weightSum)
	return &IssueLifecycleMetrics{
		Churn:           cloneFloat64Ptr(&churn),
		SnapshotCount:   len(items),
		TransitionCount: transitions,
	}
}

func issueSnapshotDelta(left, right IssueSnapshot) (float64, bool) {
	var (
		total   float64
		weights float64
	)

	if len(left.Embedding) > 0 && len(left.Embedding) == len(right.Embedding) {
		total += (1 - vectors.CosineSimilarity(left.Embedding, right.Embedding)) * 0.7
		weights += 0.7
	}

	if delta, ok := issueTagProfileDelta(left.TagScores, right.TagScores); ok {
		total += delta * 0.3
		weights += 0.3
	}

	if weights == 0 {
		return 0, false
	}
	return clamp01(total / weights), true
}

func issueTagProfileDelta(left, right []TagRelevance) (float64, bool) {
	if len(left) == 0 && len(right) == 0 {
		return 0, false
	}

	leftVector := make(map[string]float64, len(left))
	for _, score := range left {
		leftVector[score.Tag] = score.Relevance
	}

	rightVector := make(map[string]float64, len(right))
	for _, score := range right {
		rightVector[score.Tag] = score.Relevance
	}

	var (
		dot  float64
		magL float64
		magR float64
	)
	for tag, value := range leftVector {
		dot += value * rightVector[tag]
		magL += value * value
	}
	for _, value := range rightVector {
		magR += value * value
	}

	if magL == 0 || magR == 0 {
		return 1, true
	}

	similarity := dot / (math.Sqrt(magL) * math.Sqrt(magR))
	return clamp01(1 - similarity), true
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
