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
	DefaultDriftTagDeltaFloor = 0.3

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

	tagNames, tagEmbeddings := tagDataFromIssues(storeIssues, storeTags)
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
// candidates, each filtered by the delta floor, ranked by |Δ|, and capped.
func splitDriftTags(rows []issuemath.TagDrift) (spurious, missing []DebugDriftTag) {
	for _, row := range rows {
		if absFloat(row.Delta) < DefaultDriftTagDeltaFloor {
			continue
		}
		tag := DebugDriftTag{
			Tag:    row.Tag,
			Anchor: round3(row.Anchor),
			Ridge:  round3(row.Ridge),
			Delta:  round3(row.Delta),
		}
		switch {
		case row.Anchored && row.Delta < 0:
			spurious = append(spurious, tag)
		case !row.Anchored && row.Delta > 0:
			missing = append(missing, tag)
		}
	}
	// Spurious: most negative Δ first. Missing: most positive Δ first.
	slices.SortStableFunc(spurious, func(a, b DebugDriftTag) int {
		if c := cmp.Compare(a.Delta, b.Delta); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})
	slices.SortStableFunc(missing, func(a, b DebugDriftTag) int {
		if c := cmp.Compare(b.Delta, a.Delta); c != 0 {
			return c
		}
		return cmp.Compare(a.Tag, b.Tag)
	})
	if len(spurious) > maxDriftTags {
		spurious = spurious[:maxDriftTags]
	}
	if len(missing) > maxDriftTags {
		missing = missing[:maxDriftTags]
	}
	return spurious, missing
}
