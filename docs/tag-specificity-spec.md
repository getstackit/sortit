# Tag Specificity Score — Spec

## Overview

Add a continuous specificity score (0–1) to every tag, replacing the binary `IsGenericBucketTag()` hardcoded list. Specificity should capture how narrowly a tag scopes a problem domain. It should be catalog-relative, deterministic, and stable enough to persist and use across ranking and UI surfaces.

This document is a narrower sub-spec. The broader scoring architecture now lives in [scoring-spec.md](./scoring-spec.md). Where the two differ, the broader scoring spec wins, especially on the requirement that persisted scores be deterministic and explainable.

## Current Problems

The current implementation direction is not strong enough for a persisted score:

- The embedding-side score is based on random k-means initialization, so rescoring the same catalog can produce different outputs.
- The current centroid-distance heuristic does not cleanly match the concept of specificity. It measures "far from a chosen center" more than "narrow and discriminative."
- The current service code can misalign embedding subscores with tags if the catalog order differs from the storage order.
- An LLM-blended final score is difficult to treat as deterministic, explainable infrastructure.

This spec updates the target design accordingly.

## Scoring Model

### Decision Summary

For the next implementation, Sortit should treat `specificity` as a deterministic, persisted score computed from tag embeddings and stable catalog-relative rules.

The preferred v1 design is:

- `specificity` is derived from a deterministic embedding-neighborhood score.
- `specificity_llm` may still be stored as an audit/debug hint, but it should not be part of the canonical persisted `specificity` used by ranking or display logic.
- If workspace-level tag broadness from issue usage is needed later, that should be a separate signal such as `tag_coverage` or `genericity`, not silently folded into `specificity`.

### Canonical Score

The canonical score should measure local semantic rarity, not centroid outlierness.

For a tag embedding `e_i`:

1. Find the `m` nearest neighbor tags by cosine distance, excluding the tag itself.
2. Compute the mean cosine distance to those neighbors:

```
local_distance_i = mean(cosine_distance(e_i, nn_j))
```

3. Convert that raw value to a catalog-relative percentile rank:

```
specificity_i = percentile_rank(local_distance_i among all scored tags)
```

Interpretation:

- Low mean distance to nearby tags means the tag lives in a dense semantic neighborhood, so it is broader or less discriminative.
- High mean distance to nearby tags means the tag sits in a sparse neighborhood, so it is more specific.

This is a better fit for the concept than distance from a random cluster centroid.

### Parameters

- Embedding distance:
  cosine distance
- Neighbor count:
  `m = min(8, n-1)`
- Small-catalog fallback:
  if `n < 4`, return `0` for all tags and rely on the score remaining unset or neutral until the catalog is large enough to support a relative measure
- Output range:
  `0..1`

### Why Nearest-Neighbor Density

This approach is preferred because it is:

- deterministic for the same catalog
- easy to explain
- insensitive to random centroid initialization
- local rather than cluster-shape dependent
- naturally catalog-relative

### Optional LLM Audit Signal

The existing `specificity_llm` column can still be used as an optional audit signal:

- computed from tag name and description, optionally with catalog context
- shown in debug UIs if useful
- used for offline comparison against the deterministic score
- not blended into canonical persisted `specificity`

This preserves room for semantic judgment without making ranking depend on a nondeterministic subscore.

### Explicit Non-Decision

This spec does not define workspace usage prevalence as part of specificity.

If the product needs a notion like "generic bucket tag" based on how many issues a tag covers, that should be introduced as a separate usage-derived signal. A tag can be semantically specific and still have high usage in one workspace.

### Computation Lifecycle

- **Separate pass**: Specificity scoring is NOT part of the tag creation/embedding flow. Tags are created and embedded first via `EnsureStoredTags()`. Specificity is computed as a follow-up step.
- **Event-driven updates**: A tag's specificity is recomputed when an issue is created/updated that touches that tag (triggering the pass for affected tags).
- **Post-merge recalculation**: After a tag merge, the canonical tag's specificity is always recalculated.
- **No cascade on new tag creation**: Adding a new tag does not trigger re-scoring of existing tags. Drift is accepted. Users can manually trigger a full re-score if desired.
- **Manual full re-score**: Expose an operation to recalculate specificity for all tags (useful after bulk imports or significant catalog changes).

## Schema Changes

Add three columns to the `tags` table:

```sql
ALTER TABLE tags ADD COLUMN specificity REAL;
ALTER TABLE tags ADD COLUMN specificity_llm REAL;
ALTER TABLE tags ADD COLUMN specificity_embedding REAL;
ALTER TABLE tags ADD COLUMN specificity_computed_at TIMESTAMPTZ;
```

Separate columns (not JSONB) for queryability and indexability. All nullable — tags created before migration will have NULL until scored.

## Replacing `IsGenericBucketTag()`

The hardcoded list (`api, backend, client, frontend, server, ui, ux`) and all code that references it is eliminated entirely:

- **`genericBucketPenalty()` in `tag_quality.go`**: Replace with a specificity-based penalty (e.g. `(1 - specificity) * 0.04`).
- **50% opacity dimming in `tag-relevance-bars.tsx`**: Replace the `IsGenericBucketTag` check with `specificity < 0.3` threshold.
- **`buildSpecificTagSuggestions()` in `tag-quality.ts`**: Replace the bucket tag check with a specificity threshold check (see Recommendations section).

## Impact Surfaces

### 1. Display Tag Selection (`displayTags()`)

Current: picks top 3 tags by relevance, filters below 0.2.

New: compute a display score blending relevance and specificity equally:

```
displayScore = relevance * 0.5 + specificity * 0.5
```

Pick top 3 by `displayScore`, still filtering raw relevance below 0.2. This strongly favors specific tags — an issue card should communicate what's unique about it, not restate broad categories.

### 2. Search/Filter Ranking

When filtering issues by a tag:
- **Specific tag filter** (specificity ≥ 0.5): Boost issues where the matched tag has high relevance. Standard behavior.
- **Low-specificity tag filter** (specificity < 0.5): Deprioritize issues that lack co-occurring specific tags. Issues with only the low-specificity tag and no specific tags rank lower than issues that also carry specific tags. This encourages users toward more precise filtering.

### 3. Merge Candidate Scoring

The consolidation scoring formula is unchanged. Instead, add a **visual warning** in the UI when both the canonical and alias tags have high specificity (both ≥ 0.7). The warning indicates that merging two highly-specific tags is a bigger semantic decision and should be done carefully.

### 4. Person Tag Profiles

When aggregating tag scores to build a person's expertise profile, weight each tag's contribution by its specificity:

```
weighted_relevance = relevance * specificity
```

This means expertise in specific areas (e.g. "safari-flexbox-gap") contributes more signal than expertise in broad areas (e.g. "frontend"). Generalists will show a flatter profile, which accurately reflects breadth vs depth.

### 5. Recommendations — Specificity Ladder

Replace the current `buildSpecificTagSuggestions()` (which only works for hardcoded bucket tags) with a richer "specificity ladder":

- For **any selected tag**, show both more-specific and more-generic related tags.
- "More specific": find semantically similar tags (cosine similarity ≥ 0.5) with higher specificity, sorted by specificity descending.
- "More generic": find semantically similar tags with lower specificity, sorted by specificity ascending.
- Use pure similarity + specificity ordering (no LLM-identified hierarchy).
- Apply to any tag below a specificity threshold of 0.4 for proactive "more specific alternatives" suggestions.

## API Changes

Extend the `GET /tags` response to include specificity scores:

```json
{
  "tags": [
    {
      "name": "safari-flexbox-gap",
      "description": "...",
      "createdAt": "...",
      "embedding": [...],
      "specificity": 0.92,
      "specificityLlm": 0.95,
      "specificityEmbedding": 0.85
    }
  ]
}
```

All three scores are exposed for transparency/debugging. `specificity` may be `null` for tags that haven't been scored yet.

Implementation note:

- `specificity` is the canonical deterministic score.
- `specificityEmbedding` should reflect the same canonical deterministic computation unless and until the schema is simplified.
- `specificityLlm` is optional debug/audit data and should not be treated as part of the ranking contract.

## UI Changes

### Tag Detail Page

Add a **gauge visualization** showing where the tag falls on the Generic ↔ Specific spectrum:
- Horizontal gauge/meter from "Generic" (left, 0) to "Specific" (right, 1).
- The tag's specificity score is marked on the gauge.
- Tooltip shows the sub-component scores (LLM and embedding).
- Placed in the existing description card area.

### Tag Map Page

Encode specificity as **node size** in the semantic projection (2D PCA) graph:
- More specific tags → larger nodes.
- Generic tags → smaller nodes.
- Provides spatial intuition about which tags carry the most discriminative power.
- Node size range: e.g. 6px (specificity=0) to 20px (specificity=1), linear interpolation.

### Merge Consolidation UI

When both tags in a merge suggestion have specificity ≥ 0.7, display a warning annotation (e.g. amber icon + "Both tags are highly specific — merging may lose meaningful distinction").

### Tag Relevance Bars

Replace the hardcoded 50% opacity dimming of generic bucket tags with specificity-based opacity:
- Tags with specificity < 0.3 get dimmed (existing visual treatment).
- All other tags render at full opacity.

## Migration Strategy

Existing tags will have NULL specificity columns after the schema migration. The system should:

1. Gracefully handle NULL specificity everywhere (treat as 0.5 or skip specificity weighting when NULL).
2. Provide a one-time "Score all tags" operation that computes specificity for every existing tag.
3. This operation can be triggered manually or run automatically on first access after migration.

## Embedding Specificity Implementation Notes

- Run server-side in Go.
- Input: all tag embeddings (1536-dim float64 vectors).
- Build a stable list of scored tags in one explicit order and preserve name-to-score alignment all the way through persistence.
- For each tag, compute cosine distance to every other scored tag, then select the `m` nearest neighbors.
- Convert mean neighbor distance into a percentile-ranked `0..1` score across the catalog.
- Tags without embeddings receive `NULL` canonical specificity rather than a synthetic zero when possible. If the current schema path requires a value, treat them as neutral at read time rather than broad at write time.
- Small dataset (<200 tags) means `O(n^2)` pairwise distance computation is acceptable and simpler than approximate-neighbor machinery.

## Candidate Implementation Sketch

Given tags `t_1..t_n` with embeddings:

1. Filter to tags with embeddings and sort by normalized tag name.
2. For each scored tag `i`, compute pairwise cosine distances to every other scored tag.
3. Sort the distances ascending and keep the first `m`.
4. Compute:

```text
raw_i = mean(nearest_m_distances_i)
specificity_i = percentile_rank(raw_i)
```

5. Persist by tag name, not by positional assumptions across reordered catalog slices.

## Required Tests

The implementation should not ship without fixture-based tests for:

1. Reproducibility
   The same tag catalog and embeddings produce the same scores across repeated runs.
2. Order invariance
   Reordering the input slice does not change which score is assigned to which tag.
3. Dense vs sparse neighborhoods
   A tag in a sparse neighborhood scores higher than a tag in a dense cluster.
4. Missing embeddings
   Tags without embeddings are handled gracefully and do not corrupt score alignment.
5. Small catalogs
   Catalogs with `0`, `1`, `2`, and `3` scored tags produce defined, documented behavior.
6. Backfill behavior
   A full rescore of an existing catalog updates every tag deterministically.

## Open Questions

- Should small catalogs leave specificity as `NULL` instead of forcing a neutral numeric value?
- Should percentile ranking use average-rank handling for ties or a simpler normalized index?
- Should the read path expose an additional `specificityConfidence` later for very small catalogs?
