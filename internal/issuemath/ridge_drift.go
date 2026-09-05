package issuemath

import (
	"math"

	"sortit/internal/issues"
	"sortit/internal/vectors"
)

const (
	// driftZDeltaMinObservations is the minimum number of per-tag delta
	// observations (issues that produced a row for the tag) required before a
	// z-score is emitted. The per-tag mean and variance drive the
	// standardization; from a handful of issues they are too noisy to trust, and
	// a z-score built on them is worse than none — ranking then falls back to the
	// raw-delta rule. 20 is the conventional small-sample floor for treating a
	// mean/variance estimate as usable.
	driftZDeltaMinObservations = 20
	// driftZDeltaStdEpsilon is the smallest per-tag delta standard deviation that
	// yields a usable z-score. When a tag's delta barely varies across the corpus
	// (every issue agrees), (delta − mean)/std explodes on floating-point noise
	// and manufactures huge z-scores from nothing, so below this floor the
	// z-score is omitted rather than emitted.
	driftZDeltaStdEpsilon = 1e-6
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
	// ZDelta standardizes Delta within the corpus: (Delta − mean_k) / std_k over
	// every issue that produced a row for this tag. Raw Delta is not comparable
	// across tags because the ridge penalty differs per tag (scored vs unscored λ
	// swing by different amounts by construction); the z-score is. It is nil when
	// the tag has too few observations or near-degenerate corpus variance to
	// standardize honestly (see driftZDeltaMinObservations / driftZDeltaStdEpsilon)
	// — callers then fall back to the raw Delta rule. Optional-numeric shape
	// mirrors domain.TagRelevance.Negation.
	ZDelta *float64
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
	embDim := ridgeEmbeddingDim(issueEmbeddings, tagEmbeddings)
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

	standardizeTagDeltas(out)
	return out
}

// standardizeTagDeltas fills in each TagDrift.ZDelta by standardizing the raw
// delta within its tag's corpus distribution. It is a second pass over the
// already-computed per-issue results (the data is all in memory, which is
// simpler and less error-prone than streaming moments): first accumulate each
// tag's delta count, sum, and sum-of-squares across every row, then attach
// zDelta = (delta − mean_k) / std_k to each row whose tag has enough
// observations and non-degenerate variance. Tags failing either guard keep a
// nil ZDelta so callers fall back to the raw-delta rule.
func standardizeTagDeltas(drifts []IssueDrift) {
	type deltaStats struct {
		count int
		sum   float64
		sumSq float64
	}
	stats := make(map[string]*deltaStats)
	for i := range drifts {
		for _, td := range drifts[i].Tags {
			s := stats[td.Tag]
			if s == nil {
				s = &deltaStats{}
				stats[td.Tag] = s
			}
			s.count++
			s.sum += td.Delta
			s.sumSq += td.Delta * td.Delta
		}
	}
	for i := range drifts {
		for j := range drifts[i].Tags {
			td := &drifts[i].Tags[j]
			s := stats[td.Tag]
			if s == nil || s.count < driftZDeltaMinObservations {
				continue
			}
			mean := s.sum / float64(s.count)
			// Population variance; sumSq/n − mean² can go slightly negative from
			// rounding when the true variance is ~0, so clamp before the sqrt.
			variance := s.sumSq/float64(s.count) - mean*mean
			if variance < 0 {
				variance = 0
			}
			std := math.Sqrt(variance)
			if std < driftZDeltaStdEpsilon {
				continue
			}
			z := (td.Delta - mean) / std
			td.ZDelta = &z
		}
	}
}
