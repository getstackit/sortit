# Tag Specificity Score — Spec

## Overview

Add a continuous specificity score (0–1) to every tag, replacing the binary `IsGenericBucketTag()` hardcoded list. Specificity captures how narrowly a tag scopes a problem domain, blending two signals: an LLM semantic judgment and a cluster-relative embedding distance. The score is intrinsic to the tag (not contextual per-issue) and is stored as a static property, recomputed on demand.

## Scoring Model

### Two Sub-Scores

**LLM Score (weight: 0.7)**
- Computed by the AI analyzer when specificity is scored.
- The LLM receives the tag's name, description, and the full existing tag catalog as context.
- It returns a float 0–1 representing how semantically narrow/precise the tag is.
- Full catalog context allows relative reasoning (e.g. "safari" is more specific when "browser" exists).

**Embedding Distance Score (weight: 0.3)**
- Cluster-relative distance using k-means on all tag embeddings (1536-dim vectors).
- `k = floor(sqrt(n))` where n is the number of tags. Adapts as catalog grows.
- For each tag, compute cosine distance from its assigned cluster centroid.
- Normalize distances to 0–1 within each cluster (max distance in cluster = 1.0).
- **Outlier cap**: Clamp the embedding sub-score at 0.8 to avoid pathological cases where a poorly-defined tag scores maximally specific just because it doesn't fit any cluster.

**Final Score**: `specificity = llm_score * 0.7 + embedding_score * 0.3`

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
- **Generic tag filter** (specificity < 0.5): Deprioritize issues that lack co-occurring specific tags. Issues with only the generic tag and no specific tags rank lower than issues that also carry specific tags. This encourages users toward more precise filtering.

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

## K-Means Implementation Notes

- Run server-side in Go.
- Input: all tag embeddings (1536-dim float64 vectors).
- `k = floor(sqrt(n))`, minimum k=2.
- Standard k-means with random initialization, 50 max iterations, convergence threshold 1e-6.
- After clustering, compute each tag's cosine distance from its cluster centroid.
- Normalize per-cluster to 0–1 range, cap at 0.8.
- Small dataset (<200 tags) means performance is not a concern.
