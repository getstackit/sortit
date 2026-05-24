package issueanalytics

import (
	"math"
	"time"

	"sortit/internal/issues"
)

func DeriveIssueLifecycleMetrics(raw string, posts []issues.IssuePost, snapshots []issues.IssueSnapshot) *issues.IssueLifecycleMetrics {
	return DeriveIssueLifecycleMetricsAt(raw, posts, nil, snapshots, time.Now().UTC())
}

func DeriveIssueLifecycleMetricsAt(raw string, posts []issues.IssuePost, links []issues.IssueLink, snapshots []issues.IssueSnapshot, now time.Time) *issues.IssueLifecycleMetrics {
	metrics := issues.ComputeIssueLifecycleMetrics(snapshots)
	metrics = AttachIssueMaturity(raw, posts, metrics)
	return AttachIssueVelocity(posts, links, metrics, now)
}

func AttachIssueMaturity(raw string, posts []issues.IssuePost, metrics *issues.IssueLifecycleMetrics) *issues.IssueLifecycleMetrics {
	refinementCount, progressCount := CountIssuePostKinds(posts)
	contentConfidence := ComputeContentConfidence(raw)
	stabilitySignal := issueStabilitySignal(metrics)
	maturity := computeIssueMaturity(contentConfidence, refinementCount, progressCount, stabilitySignal)

	if metrics == nil {
		metrics = &issues.IssueLifecycleMetrics{}
	}
	metrics.Maturity = cloneFloat64Ptr(&maturity)
	metrics.RefinementCount = refinementCount
	metrics.ProgressCount = progressCount
	return metrics
}

func CountIssuePostKinds(posts []issues.IssuePost) (refinementCount int, progressCount int) {
	for _, post := range posts {
		switch issuePostKind(post) {
		case issuePostKindProgress:
			progressCount++
		case issuePostKindRefinement:
			refinementCount++
		}
	}
	return refinementCount, progressCount
}

func computeIssueMaturity(contentConfidence float64, refinementCount int, progressCount int, stabilitySignal float64) float64 {
	refinementSignal := 1 - math.Exp(-float64(refinementCount)/2.0)
	progressSignal := 1 - math.Exp(-float64(progressCount)/2.0)
	activitySignal := 0.75*refinementSignal + 0.25*progressSignal

	return clamp01(
		0.10 +
			0.35*activitySignal +
			0.20*contentConfidence +
			0.35*stabilitySignal,
	)
}

func issueStabilitySignal(metrics *issues.IssueLifecycleMetrics) float64 {
	if metrics != nil && metrics.Stability != nil {
		return clamp01(*metrics.Stability)
	}
	return 0.35
}
