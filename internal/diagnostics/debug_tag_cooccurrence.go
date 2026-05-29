package diagnostics

import (
	"context"
	"strings"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/tagcooccurrence"
)

// DebugTagCooccurrenceResult is the response body for
// GET /api/v1/debug/tag-cooccurrence.
type DebugTagCooccurrenceResult struct {
	IssueCount int                                 `json:"issueCount"`
	TagCount   int                                 `json:"tagCount"`
	Tag        string                              `json:"tag,omitempty"`
	Stats      *DebugTagCooccurrenceTagStats       `json:"stats,omitempty"`
	Top        []DebugTagCooccurrenceCorrelatedTag `json:"top,omitempty"`
	Tags       []DebugTagCooccurrenceTagSummary    `json:"tags,omitempty"`
}

// DebugTagCooccurrenceTagStats wraps the per-tag projection for a specific
// query tag and exposes the top anti-correlated neighbors.
type DebugTagCooccurrenceTagStats struct {
	Tag         string                              `json:"tag"`
	Count       int                                 `json:"count"`
	TopNegative []DebugTagCooccurrenceCorrelatedTag `json:"topNegative"`
}

// DebugTagCooccurrenceCorrelatedTag is a single neighbor in the
// co-occurrence projection.
type DebugTagCooccurrenceCorrelatedTag struct {
	Tag              string  `json:"tag"`
	JointCount       int     `json:"jointCount"`
	BaseProb         float64 `json:"baseProb"`
	ConditionalProb  float64 `json:"conditionalProb"`
	ImplicitNegative float64 `json:"implicitNegative"`
}

// DebugTagCooccurrenceTagSummary is included when the request does not
// specify a tag — it lists every tag's count so callers can pick targets.
type DebugTagCooccurrenceTagSummary struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// DebugTagCooccurrenceHandler returns the corpus-wide co-occurrence
// projection. When a *tagcooccurrence.Cache is wired, the projection is
// served from the cache (recomputed only on corpus revision bumps);
// otherwise the handler falls back to walking issues directly each call.
type DebugTagCooccurrenceHandler struct {
	Store issues.Store
	Cache *tagcooccurrence.Cache
	TopK  int
}

const defaultDebugTagCooccurrenceTopK = 10

// Handle returns the projection. If tag is non-empty, the response includes
// per-tag detail plus top-K anti-correlated neighbors. If tag is empty, the
// response lists every tag's count so the caller can choose a target.
func (h DebugTagCooccurrenceHandler) Handle(ctx context.Context, tag string) (DebugTagCooccurrenceResult, error) {
	var stats tagcooccurrence.Stats
	if h.Cache != nil {
		cached, err := h.Cache.Current(ctx)
		if err != nil {
			return DebugTagCooccurrenceResult{}, err
		}
		stats = cached
	} else {
		var storeIssues []issues.Issue
		if h.Store != nil {
			items, err := h.Store.List(ctx)
			if err != nil {
				return DebugTagCooccurrenceResult{}, err
			}
			storeIssues = items
		}
		stats = tagcooccurrence.Compute(storeIssues)
	}

	topK := h.TopK
	if topK <= 0 {
		topK = defaultDebugTagCooccurrenceTopK
	}

	out := DebugTagCooccurrenceResult{
		IssueCount: stats.IssueCount,
		TagCount:   stats.TagCount,
	}

	tag = strings.TrimSpace(tag)
	if tag == "" {
		out.Tags = make([]DebugTagCooccurrenceTagSummary, 0, len(stats.Tags))
		for _, t := range stats.Tags {
			out.Tags = append(out.Tags, DebugTagCooccurrenceTagSummary{
				Tag:   t.Tag,
				Count: t.Count,
			})
		}
		return out, nil
	}

	normalized := domain.NormalizeTagName(tag)
	out.Tag = normalized
	tagStats, ok := stats.LookupTag(normalized)
	if !ok {
		return out, nil
	}

	top := make([]DebugTagCooccurrenceCorrelatedTag, 0, topK)
	for _, pair := range tagStats.Pairs {
		if len(top) >= topK {
			break
		}
		if pair.ImplicitNegative <= 0 {
			break
		}
		top = append(top, DebugTagCooccurrenceCorrelatedTag{
			Tag:              pair.Tag,
			JointCount:       pair.JointCount,
			BaseProb:         pair.BaseProb,
			ConditionalProb:  pair.ConditionalProb,
			ImplicitNegative: pair.ImplicitNegative,
		})
	}

	out.Stats = &DebugTagCooccurrenceTagStats{
		Tag:         tagStats.Tag,
		Count:       tagStats.Count,
		TopNegative: top,
	}
	out.Top = top
	return out, nil
}
