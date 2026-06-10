package diagnostics

import (
	"context"
	"math"
	"sort"

	"sortit/internal/centering"
	"sortit/internal/domain"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

// DefaultRidgeLambda is the shrinkage default for the debug endpoint.
// Small enough to let the embedding shift relevance noticeably; large
// enough that the result stays anchored to the analyzer's beliefs.
const DefaultRidgeLambda = 0.5

// DebugRidgeScoreResult is the response body for
// GET /api/v1/debug/issues/{id}/ridge.
type DebugRidgeScoreResult struct {
	IssueID string             `json:"issueId"`
	Lambda  float64            `json:"lambda"`
	Tags    []DebugRidgeTagRow `json:"tags"`
}

// DebugRidgeTagRow pairs the existing signed anchor with the refined
// ridge output so callers can see how much the regression moved each
// tag.
type DebugRidgeTagRow struct {
	Tag    string  `json:"tag"`
	Anchor float64 `json:"anchor"`
	Ridge  float64 `json:"ridge"`
	Delta  float64 `json:"delta"`
}

// DebugRidgeScoreHandler runs the anchored ridge regression from
// docs/math-evolution.md against one issue.
type DebugRidgeScoreHandler struct {
	Store   issues.Store
	Catalog *tags.CatalogService
	Lambda  float64
	// Centering provides the revision-cached corpus means; the ridge solve
	// runs in the same centered space as the rest of the math layer. Nil
	// leaves the vectors uncentered (unit-normalized only).
	Centering *centering.Cache
}

// Handle computes the refined scores. Returns an empty result when the
// issue lacks an embedding or no catalog tag has an embedding (e.g.,
// fresh database).
func (h DebugRidgeScoreHandler) Handle(ctx context.Context, issueID string) (DebugRidgeScoreResult, error) {
	issue, err := h.Store.Get(ctx, issueID)
	if err != nil {
		return DebugRidgeScoreResult{}, err
	}
	stored, err := h.Catalog.StoredTags(ctx)
	if err != nil {
		return DebugRidgeScoreResult{}, err
	}

	// Filter to catalog tags that have embeddings.
	type catalogTag struct {
		Name      string
		Embedding []float64
	}
	keep := make([]catalogTag, 0, len(stored))
	for _, t := range stored {
		if len(t.Embedding) == 0 {
			continue
		}
		keep = append(keep, catalogTag{Name: t.Name, Embedding: t.Embedding})
	}

	lambda := h.Lambda
	if lambda <= 0 {
		lambda = DefaultRidgeLambda
	}

	out := DebugRidgeScoreResult{
		IssueID: issueID,
		Lambda:  lambda,
	}
	if len(keep) == 0 || len(issue.Embedding) == 0 {
		return out, nil
	}

	means, err := h.Centering.Current(ctx)
	if err != nil {
		return DebugRidgeScoreResult{}, err
	}

	// Anchor: signed tag relevance (positive minus negation magnitude).
	// Tag rows and the issue embedding are centered with the corpus means
	// and unit-normalized, as ComputeRidgeScores expects.
	anchorByTag := signedAnchorByTag(issue.TagScores)
	tagEmbeddings := make([][]float64, len(keep))
	anchor := make([]float64, len(keep))
	for i, t := range keep {
		tagEmbeddings[i] = issuemath.CenterVector(t.Embedding, means.Tag)
		anchor[i] = anchorByTag[domain.NormalizeTagName(t.Name)]
	}

	refined := issuemath.ComputeRidgeScores(tagEmbeddings, issuemath.CenterVector(issue.Embedding, means.Issue), anchor, lambda)
	if refined == nil {
		return out, nil
	}

	out.Tags = make([]DebugRidgeTagRow, len(keep))
	for i, t := range keep {
		out.Tags[i] = DebugRidgeTagRow{
			Tag:    t.Name,
			Anchor: round3(anchor[i]),
			Ridge:  round3(refined[i]),
			Delta:  round3(refined[i] - anchor[i]),
		}
	}

	// Sort by descending |delta| so the rows that moved most surface first.
	sort.SliceStable(out.Tags, func(i, j int) bool {
		ai := absFloat(out.Tags[i].Delta)
		aj := absFloat(out.Tags[j].Delta)
		if ai != aj {
			return ai > aj
		}
		return out.Tags[i].Tag < out.Tags[j].Tag
	})
	return out, nil
}

// signedAnchorByTag derives the per-tag prior the ridge regression
// shrinks toward. Signed = Relevance - (Negation or 0); matches the
// math-evolution doc's r_i = r_i⁺ − r_i⁻ formulation.
func signedAnchorByTag(scores []domain.TagRelevance) map[string]float64 {
	out := make(map[string]float64, len(scores))
	for _, score := range scores {
		negation := 0.0
		if score.Negation != nil {
			negation = *score.Negation
		}
		out[domain.NormalizeTagName(score.Tag)] = score.Relevance - negation
	}
	return out
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}
