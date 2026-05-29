package issuexray

import (
	"context"
	"errors"
	"time"

	"sortit/internal/issues"
	"sortit/internal/tagcooccurrence"
)

// ErrIssueNotFound is returned when the requested issue ID isn't in the store.
var ErrIssueNotFound = errors.New("issue not found")

// IssueStore is the read interface the handler needs.
type IssueStore interface {
	Get(ctx context.Context, id string) (issues.Issue, error)
}

// TagReader supplies catalog tags so per-tag specificity can be looked up.
type TagReader interface {
	StoredTags(ctx context.Context) ([]issues.Tag, error)
}

// Cooccurrence supplies the current corpus-wide projection. May be nil
// (the scorecard degrades to nil Coherence).
type Cooccurrence interface {
	Current(ctx context.Context) (tagcooccurrence.Stats, error)
}

// Handler computes the per-issue X-ray scorecard.
type Handler struct {
	Issues       IssueStore
	Tags         TagReader
	Cooccurrence Cooccurrence
	Clock        func() time.Time
}

// Get computes the scorecard for the issue with the given ID.
func (h *Handler) Get(ctx context.Context, id string) (IssueXRay, error) {
	if h == nil || h.Issues == nil {
		return IssueXRay{}, ErrIssueNotFound
	}
	issue, err := h.Issues.Get(ctx, id)
	if err != nil {
		return IssueXRay{}, err
	}

	var tagSpec map[string]float64
	if h.Tags != nil {
		stored, err := h.Tags.StoredTags(ctx)
		if err != nil {
			return IssueXRay{}, err
		}
		tagSpec = buildSpecificityMap(stored)
	}

	var stats *tagcooccurrence.Stats
	if h.Cooccurrence != nil {
		s, err := h.Cooccurrence.Current(ctx)
		if err != nil {
			return IssueXRay{}, err
		}
		stats = &s
	}

	return Compute(issue, tagSpec, stats, h.now()), nil
}

func (h *Handler) now() time.Time {
	if h.Clock != nil {
		return h.Clock()
	}
	return time.Now().UTC()
}

func buildSpecificityMap(stored []issues.Tag) map[string]float64 {
	out := make(map[string]float64, len(stored))
	for _, t := range stored {
		if t.Specificity == nil {
			continue
		}
		out[t.Name] = *t.Specificity
	}
	return out
}
