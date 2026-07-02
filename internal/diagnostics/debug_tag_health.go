package diagnostics

import (
	"cmp"
	"context"
	"slices"

	"sortit/internal/centering"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/tags"
)

// Drift thresholds are hyperparameters (docs/math-evolution.md §11) — surfaced
// in the response so they are visible, and named so they are tunable once an
// eval harness exists.
const (
	// DefaultDriftCosineThreshold flags an issue when the cosine between its
	// embedding-derived loading f and the analyzer anchor r falls below it.
	// DriftCosine is in [-1, 1]; 1.0 means the geometry agrees with the tagging
	// (even under uniform shrinkage), near-0/negative means genuine
	// disagreement. 0.5 is a conservative "meaningfully diverging" cut.
	DefaultDriftCosineThreshold = 0.5
	// DefaultDriftTagDeltaFloor is the minimum |f_k − r_k| for a tag to surface
	// as a spurious or missing candidate, so benign uniform shrinkage does not.
	// Retained as a secondary guard even when a z-score is available: a tag with
	// near-zero corpus variance could otherwise turn a tiny absolute delta into a
	// huge z-score, so a candidate must clear this raw floor regardless.
	DefaultDriftTagDeltaFloor = 0.3
	// DefaultDriftZDeltaThreshold is the minimum |zDelta| for a tag with a valid
	// z-score to surface as a candidate. Raw per-tag deltas are not comparable
	// across tags because the ridge penalty differs per tag (scored 0.5 vs
	// unscored 0.05 in the drift regime), so a loosely-penalized unscored tag
	// swings wider than an anchored one by construction; the z-score standardizes
	// that away. 2.0 (≈ two standard deviations) is a conventional "clearly
	// outside the normal spread" cut. Tags without a valid z-score fall back to
	// the raw DefaultDriftTagDeltaFloor rule alone.
	DefaultDriftZDeltaThreshold = 2.0

	maxDriftIssues = 20
	maxDriftTags   = 5
)

// DebugTagHealthResult is the response body for GET /api/v1/debug/tag-health.
// It reports open issues whose AI tagging disagrees with the embedding geometry
// (docs/math-evolution.md §8.6, §8.7) — the drift-based primary tag-health
// signal, complementing the rank-1 low-R² list (which covers uncovered
// concepts rather than mis-tagging).
type DebugTagHealthResult struct {
	IssueCount      int               `json:"issueCount"`
	DriftedCount    int               `json:"driftedCount"`
	Computed        bool              `json:"computed"`
	LambdaUnscored  float64           `json:"lambdaUnscored"`
	DriftThreshold  float64           `json:"driftThreshold"`
	HighDriftIssues []DebugDriftIssue `json:"highDriftIssues"`
}

// DebugDriftIssue is one open issue whose tagging diverges from its embedding,
// with the tags most responsible split into over-claimed (spurious) and
// under-claimed (missing).
type DebugDriftIssue struct {
	ID           string             `json:"id"`
	Raw          string             `json:"raw"`
	Status       issues.IssueStatus `json:"status"`
	DriftCosine  float64            `json:"driftCosine"`
	SpuriousTags []DebugDriftTag    `json:"spuriousTags"`
	MissingTags  []DebugDriftTag    `json:"missingTags"`
}

// DebugDriftTag is one tag's contribution to an issue's drift.
type DebugDriftTag struct {
	Tag    string  `json:"tag"`
	Anchor float64 `json:"anchor"` // r_k
	Ridge  float64 `json:"ridge"`  // f_k
	Delta  float64 `json:"delta"`  // f_k − r_k
	// ZDelta is Delta standardized within the tag's corpus distribution — the
	// cross-tag-comparable ranking signal. Omitted (nil) when the tag has too
	// small a sample or too little corpus variance to standardize; the candidate
	// then ranks on Delta alone.
	ZDelta *float64 `json:"zDelta,omitempty"`
}

// DebugTagHealthHandler runs the corpus drift sweep and surfaces the issues
// whose assigned tags most disagree with their embedding geometry.
type DebugTagHealthHandler struct {
	Store   issues.Store
	Catalog *tags.CatalogService
	// Centering supplies the revision-cached corpus means so drift is measured
	// in the same space as the ranker and the per-issue ridge endpoint; nil
	// centers against freshly computed means.
	Centering *centering.Cache
}

// Handle computes per-issue drift over the corpus and returns the open issues
// flagged above the drift threshold, worst-first and deterministic.
func (h DebugTagHealthHandler) Handle(ctx context.Context) (DebugTagHealthResult, error) {
	result := DebugTagHealthResult{DriftThreshold: DefaultDriftCosineThreshold}

	var storeIssues []issues.Issue
	if h.Store != nil {
		items, err := h.Store.List(ctx)
		if err != nil {
			return DebugTagHealthResult{}, err
		}
		storeIssues = items
	}
	result.IssueCount = len(storeIssues)

	var storeTags []issues.Tag
	if h.Catalog != nil {
		stored, err := h.Catalog.StoredTags(ctx)
		if err == nil {
			storeTags = stored
		}
	}

	tagNames, tagEmbeddings := issuemath.TagDataFromIssues(storeIssues, storeTags)
	issueEmbeddings := make(map[string][]float64, len(storeIssues))
	for _, issue := range storeIssues {
		if len(issue.Embedding) > 0 {
			issueEmbeddings[issue.ID] = issue.Embedding
		}
	}

	// Center in the same space the ranker and the GCV cache use. With a cache,
	// use the revision-cached corpus means; without one (tests) fall back to
	// means over the inputs.
	if h.Centering != nil {
		means, err := h.Centering.Current(ctx)
		if err != nil {
			return DebugTagHealthResult{}, err
		}
		issueEmbeddings, tagEmbeddings = issuemath.CenterEmbeddingsWith(means, issueEmbeddings, tagEmbeddings)
	} else {
		issueEmbeddings, tagEmbeddings, _ = issuemath.CenterEmbeddings(issueEmbeddings, tagEmbeddings)
	}

	// Drift uses the fixed unscored penalty, matching the per-issue
	// /debug/issues/{id}/ridge endpoint — not the GCV penalty. GCV is tuned for
	// ranking and pins unscored tags toward zero, which suppresses the very
	// signal drift depends on: an embedding "voting" for an unassigned tag. The
	// fixed penalty lets unscored tags float, surfacing missing-tag candidates,
	// and keeps the corpus sweep numerically consistent with /ridge.
	lambdaUnscored := scoring.RidgeAnchorLambdaUnscored
	result.LambdaUnscored = round3(lambdaUnscored)

	drifts := issuemath.ComputeCorpusDrift(storeIssues, tagNames, issueEmbeddings, tagEmbeddings,
		scoring.RidgeAnchorLambdaScored, lambdaUnscored)
	if len(drifts) == 0 {
		return result, nil
	}
	result.Computed = true

	issueByID := make(map[string]issues.Issue, len(storeIssues))
	for _, issue := range storeIssues {
		issueByID[issue.ID] = issue
	}

	for _, d := range drifts {
		issue, ok := issueByID[d.ID]
		if !ok || issue.Status != issues.StatusOpen {
			continue
		}
		if d.DriftCosine >= DefaultDriftCosineThreshold {
			continue
		}
		spurious, missing := splitDriftTags(d.Tags)
		if len(spurious) == 0 && len(missing) == 0 {
			// Low cosine but no single tag clears the delta floor — nothing
			// actionable to attribute, so skip rather than flag a bare score.
			continue
		}
		result.HighDriftIssues = append(result.HighDriftIssues, DebugDriftIssue{
			ID:           d.ID,
			Raw:          truncateRaw(issue.Raw),
			Status:       issue.Status,
			DriftCosine:  round3(d.DriftCosine),
			SpuriousTags: spurious,
			MissingTags:  missing,
		})
	}

	// Deterministic: most-drifted (lowest cosine) first, ID tiebreak.
	slices.SortStableFunc(result.HighDriftIssues, func(a, b DebugDriftIssue) int {
		if c := cmp.Compare(a.DriftCosine, b.DriftCosine); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	result.DriftedCount = len(result.HighDriftIssues)
	if len(result.HighDriftIssues) > maxDriftIssues {
		result.HighDriftIssues = result.HighDriftIssues[:maxDriftIssues]
	}
	return result, nil
}

// splitDriftTags partitions a drift breakdown into over-claimed (spurious,
// anchored with Δ < 0) and under-claimed (missing, unanchored with Δ > 0)
// candidates, filtered by the combined rule, ranked by |zDelta| where available,
// and capped.
//
// A candidate must always clear the raw delta floor. When a tag carries a valid
// z-score (large enough sample, non-degenerate corpus variance) it must
// additionally clear the z-threshold — the z-score is the cross-tag-comparable
// signal, so it gates and ranks. Tags without a valid z-score fall back to the
// raw-delta rule alone: they still surface if past the floor, but rank below any
// z-scored candidate.
func splitDriftTags(rows []issuemath.TagDrift) (spurious, missing []DebugDriftTag) {
	for _, row := range rows {
		if absFloat(row.Delta) < DefaultDriftTagDeltaFloor {
			continue
		}
		if row.ZDelta != nil && absFloat(*row.ZDelta) < DefaultDriftZDeltaThreshold {
			continue
		}
		tag := DebugDriftTag{
			Tag:    row.Tag,
			Anchor: round3(row.Anchor),
			Ridge:  round3(row.Ridge),
			Delta:  round3(row.Delta),
			ZDelta: round3Ptr(row.ZDelta),
		}
		switch {
		case row.Anchored && row.Delta < 0:
			spurious = append(spurious, tag)
		case !row.Anchored && row.Delta > 0:
			missing = append(missing, tag)
		}
	}
	// Rank z-scored candidates ahead of raw-only ones, each by descending
	// magnitude (|zDelta| then |Δ|), tie-broken by tag name for determinism.
	slices.SortStableFunc(spurious, driftTagRank)
	slices.SortStableFunc(missing, driftTagRank)
	if len(spurious) > maxDriftTags {
		spurious = spurious[:maxDriftTags]
	}
	if len(missing) > maxDriftTags {
		missing = missing[:maxDriftTags]
	}
	return spurious, missing
}

// driftTagRank orders drift-tag candidates: those with a comparable z-score
// first (ranked by |zDelta| descending), raw-only candidates after (ranked by
// |Δ| descending), tag name as a deterministic tiebreak. Because spurious tags
// are all Δ < 0 and missing tags all Δ > 0, |Δ| recovers the old "most negative"
// / "most positive" ordering within the raw-only tail.
func driftTagRank(a, b DebugDriftTag) int {
	az, bz := a.ZDelta != nil, b.ZDelta != nil
	if az != bz {
		if az {
			return -1
		}
		return 1
	}
	if az {
		if c := cmp.Compare(absFloat(*b.ZDelta), absFloat(*a.ZDelta)); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	}
	if c := cmp.Compare(absFloat(b.Delta), absFloat(a.Delta)); c != 0 {
		return c
	}
	return cmp.Compare(a.Tag, b.Tag)
}

// round3Ptr rounds an optional z-score for the payload, preserving nil.
func round3Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := round3(*v)
	return &r
}
