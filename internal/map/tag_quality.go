package issuemap

import (
	"splat/internal/domain"
)

func genericBucketPenalty(name string) float64 {
	if domain.IsGenericBucketTag(name) {
		return 0.04
	}
	return 0
}
