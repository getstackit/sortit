package issuemap

import (
	"splat/internal/domain"
)

var genericBucketTags = map[string]struct{}{
	"api":       {},
	"backend":   {},
	"client":    {},
	"front end": {},
	"frontend":  {},
	"server":    {},
	"ui":        {},
	"ux":        {},
}

func genericBucketPenalty(name string) float64 {
	if _, ok := genericBucketTags[normalizeTagName(name)]; ok {
		return 0.04
	}
	return 0
}

func normalizeTagName(name string) string {
	return domain.NormalizeTagName(name)
}
