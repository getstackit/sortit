package issuemath

import (
	"sortit/internal/issues"
	"sortit/internal/vectors"
)

// IssueDrift holds one issue's AI/embedding drift: the global DriftCosine
// between the embedding-derived ridge loading f and the analyzer's signed
// anchor r, plus the per-tag breakdown. It is the diagnostic counterpart to
// RidgeVectors — where RidgeVectors keeps the unit loadings the ranker needs,
// IssueDrift keeps raw f and r so callers can attribute drift per tag
// (docs/math-evolution.md §5.4, §8.6, §8.7).
type IssueDrift struct {
	ID          string
	DriftCosine float64
	Tags        []TagDrift
}

// TagDrift is one tag's contribution to an issue's drift. Delta = Ridge −
// Anchor: a large negative Delta on an anchored tag flags an over-claimed
// (spurious) tag; a large positive Delta on an unanchored catalog tag flags a
// missing tag.
type TagDrift struct {
	Tag      string
	Anchor   float64 // r_k — analyzer's signed judgment (relevance − negation), 0 if unscored
	Ridge    float64 // f_k — embedding-derived loading from the ridge solve
	Delta    float64 // f_k − r_k
	Anchored bool    // the analyzer scored or negated this tag
}

// ComputeCorpusDrift solves the anchored ridge per issue over a shared tag
// matrix and returns each issue's drift. It reuses the same solver, anchor,
// and DriftCosine primitives as the per-issue debug ridge endpoint, so the two
// agree (up to the lambda the caller passes). issueEmbeddings and tagEmbeddings
// are expected already corpus-mean centered — the same space the ranker and the
// GCV cache operate in; centering is the caller's responsibility.
//
// An issue is included only when it has a usable embedding, at least one tag in
// the catalog (a non-zero anchor — otherwise there is no analyzer opinion to
// disagree with), and a non-degenerate solve. Untagged or unsolvable issues are
// not drift candidates and are omitted; status filtering is the caller's job.
// The returned slice follows the input order, so the result is deterministic.
func ComputeCorpusDrift(
	items []issues.Issue,
	tagNames []string,
	issueEmbeddings, tagEmbeddings map[string][]float64,
	lambdaScored, lambdaUnscored float64,
) []IssueDrift {
	if len(items) == 0 || len(tagNames) == 0 {
		return nil
	}
	embDim := embeddingDim(tagEmbeddings)
	if embDim == 0 {
		return nil
	}

	numTags := len(tagNames)
	tagIndex := make(map[string]int, numTags)
	for i, tag := range tagNames {
		tagIndex[tag] = i
	}
	solver := newRidgeSolver(ridgeTagMatrix(tagNames, tagEmbeddings, embDim), embDim)

	out := make([]IssueDrift, 0, len(items))
	lambdas := make([]float64, numTags)
	for _, item := range items {
		e := issueEmbeddings[item.ID]
		if len(e) != embDim || vectors.IsZero(e) {
			continue
		}

		anchor, scored := signedAnchor(item.TagScores, tagIndex, numTags)
		anchoredAny := false
		for i, s := range scored {
			if s {
				anchoredAny = true
				lambdas[i] = lambdaScored
			} else {
				lambdas[i] = lambdaUnscored
			}
		}
		if !anchoredAny {
			continue
		}

		// f is solver-owned scratch, valid only until the next solve; it is
		// fully consumed below before the loop iterates.
		f := solver.solve(e, anchor, lambdas)
		if f == nil || vectors.IsZero(f) {
			continue
		}

		drift := IssueDrift{ID: item.ID, DriftCosine: DriftCosine(f, anchor)}
		for k := range numTags {
			if f[k] == 0 && anchor[k] == 0 {
				continue
			}
			drift.Tags = append(drift.Tags, TagDrift{
				Tag:      tagNames[k],
				Anchor:   anchor[k],
				Ridge:    f[k],
				Delta:    f[k] - anchor[k],
				Anchored: scored[k],
			})
		}
		out = append(out, drift)
	}
	return out
}
