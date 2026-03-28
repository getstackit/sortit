package issueanalytics

import "splat/internal/issues"

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func issuePostKind(post issues.IssuePost) string {
	if post.Kind != "" {
		return post.Kind
	}
	if post.Sequence == 1 {
		return "report"
	}
	return "refinement"
}
