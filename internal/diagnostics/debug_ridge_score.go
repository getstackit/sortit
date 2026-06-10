package diagnostics

import (
	"context"
	"math"
	"sort"

	"sortit/internal/centering"
	"sortit/internal/domain"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/tags"
)

// DefaultRidgeLambda is the shrinkage default for the debug endpoint,
// applied to analyzer-scored tags. Unscored catalog tags always get the
// weaker scoring.RidgeAnchorLambdaUnscored penalty.
const DefaultRidgeLambda = scoring.RidgeAnchorLambdaScored

// DebugRidgeScoreResult is the response body for
// GET /api/v1/debug/issues/{id}/ridge.
type DebugRidgeScoreResult struct {
	IssueID string `json:"issueId"`
	// Lambda is the strong per-tag penalty applied to tags the analyzer
	// scored or negated (a TagRelevance entry exists). Their ridge output
	// shrinks toward the analyzer's signed judgment.
	Lambda float64 `json:"lambda"`
	// LambdaUnscored is the weak penalty applied to catalog tags with no
	// analyzer opinion. Their anchor of 0 means "no opinion", so they
	// float almost freely toward the least-squares fit.
	LambdaUnscored float64 `json:"lambdaUnscored"`
	// DriftCosine is the cosine between the refined vector f and the
	// anchor r, restricted to tags where either is non-zero. It is the
	// direction-based replacement for ‖f − r‖, which mostly measures
	// uniform least-squares shrinkage. 1.0 means the embedding geometry
	// agrees with the analyzer's tagging direction (even if uniformly
	// shrunk); values near 0 or negative mean genuine disagreement.
	// 0 is also returned when there is nothing to compare (no embedding,
	// no tags, or a degenerate solve).
	DriftCosine float64            `json:"driftCosine"`
	Tags        []DebugRidgeTagRow `json:"tags"`
}

// DebugRidgeTagRow pairs the existing signed anchor with the refined
// ridge output so callers can see how much the regression moved each
// tag.
type DebugRidgeTagRow struct {
	Tag string `json:"tag"`
	// Anchor is the analyzer's signed judgment r_k (relevance minus
	// negation), or 0 for catalog tags the analyzer didn't score.
	Anchor float64 `json:"anchor"`
	// Anchored reports whether the analyzer expressed an opinion on this
	// tag (TagRelevance present, including negation-only). Anchored tags
	// were penalized with Lambda; unanchored ones with LambdaUnscored.
	Anchored bool `json:"anchored"`
	// Ridge is the refined score f_k from the diagonal-penalty solve.
	Ridge float64 `json:"ridge"`
	// Delta is the raw per-tag drift f_k − r_k. It is not standardized
	// against the corpus distribution of this tag's delta: Λ varies per
	// issue (it depends on which tags the analyzer scored), so computing
	// that distribution would require a fresh K×K solve for every issue
	// in the corpus on each request. Interpret large positive deltas on
	// unanchored tags as missing-tag candidates and large negative deltas
	// on anchored tags as spurious-tag candidates.
	Delta float64 `json:"delta"`
}

// DebugRidgeScoreHandler runs the anchored ridge regression from
// docs/math-evolution.md against one issue.
type DebugRidgeScoreHandler struct {
	Store   issues.Store
	Catalog *tags.CatalogService
	// Lambda overrides the penalty for analyzer-scored tags; zero or
	// negative falls back to DefaultRidgeLambda. Unscored tags always use
	// scoring.RidgeAnchorLambdaUnscored.
	Lambda float64
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
		IssueID:        issueID,
		Lambda:         lambda,
		LambdaUnscored: scoring.RidgeAnchorLambdaUnscored,
	}
	if len(keep) == 0 || len(issue.Embedding) == 0 {
		return out, nil
	}

	means, err := h.Centering.Current(ctx)
	if err != nil {
		return DebugRidgeScoreResult{}, err
	}

	// Anchor: signed tag relevance (positive minus negation magnitude).
	// Tags the analyzer scored or negated get the strong penalty; catalog
	// tags with no analyzer opinion get the weak one, so their zero anchor
	// reads as "no opinion" rather than evidence of absence. Tag rows and
	// the issue embedding are centered with the corpus means and
	// unit-normalized, as ComputeRidgeScoresDiagonal expects.
	anchorByTag := signedAnchorByTag(issue.TagScores)
	tagEmbeddings := make([][]float64, len(keep))
	anchor := make([]float64, len(keep))
	lambdas := make([]float64, len(keep))
	anchored := make([]bool, len(keep))
	for i, t := range keep {
		tagEmbeddings[i] = issuemath.CenterVector(t.Embedding, means.Tag)
		signed, scored := anchorByTag[domain.NormalizeTagName(t.Name)]
		anchor[i] = signed
		anchored[i] = scored
		if scored {
			lambdas[i] = lambda
		} else {
			lambdas[i] = scoring.RidgeAnchorLambdaUnscored
		}
	}

	refined := issuemath.ComputeRidgeScoresDiagonal(tagEmbeddings, issuemath.CenterVector(issue.Embedding, means.Issue), anchor, lambdas)
	if refined == nil {
		return out, nil
	}

	out.DriftCosine = round3(issuemath.DriftCosine(refined, anchor))
	out.Tags = make([]DebugRidgeTagRow, len(keep))
	for i, t := range keep {
		out.Tags[i] = DebugRidgeTagRow{
			Tag:      t.Name,
			Anchor:   round3(anchor[i]),
			Anchored: anchored[i],
			Ridge:    round3(refined[i]),
			Delta:    round3(refined[i] - anchor[i]),
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
// math-evolution doc's r_i = r_i⁺ − r_i⁻ formulation. Map membership doubles
// as "the analyzer expressed an opinion on this tag".
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
