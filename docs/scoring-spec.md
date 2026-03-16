# Scoring Spec

## Purpose

Define Splat's scoring model as a set of explicit, reusable signals instead of a handful of opaque blended scores.

This document is architectural. It defines what kinds of scores Splat should have, what they mean, how they should behave, and which product surfaces should consume them. It does not lock in every formula.

## Status

- Design doc, not an implementation checklist
- Applies to persisted scores, user-visible scores, and major ranking surfaces
- Companion to narrower docs such as [tag-specificity-spec.md](./tag-specificity-spec.md)

## Principles

1. Keep primitive signals separate from task-specific blends.
2. Persisted and user-visible scores must be deterministic and reproducible for the same inputs.
3. Prefer bounded monotone transforms over raw counts.
4. Treat time as context, not universal quality.
5. Separate issue quality from graph structure.
6. Every score should have a one-sentence interpretation.
7. Every major consumer should be able to explain which signals it uses.

## Non-Goals

- One universal score for every product surface
- Encoding business priority as a hidden ranking heuristic
- Replacing interpretable factor/tag structure with embedding-only ranking
- Treating every count or timestamp as an equally meaningful signal

## Model Layers

Splat scoring should be organized into three layers:

1. Primitive signals
   Stable measurements such as specificity, confidence, maturity, or authority.
2. Contextual modifiers
   Signals whose effect depends on the task, such as freshness, time decay, or resolution state.
3. Consumer blends
   Surface-specific scoring formulas for search, explore, recommendations, map weighting, and UI display.

Consumer blends may combine primitives and modifiers, but they should not redefine the primitives.

## Primitive Signals

### Similarity

Measures how related two entities are.

- Domain:
  - issue to issue
  - tag to tag
  - person to issue or person to person
- Preferred inputs:
  - factor/tag relevance structure
  - embeddings
  - graph relationships where relevant
- Rules:
  - do not collapse all similarity sources into one undocumented number
  - retain enough structure to explain whether a match came from text, tags, or links

### Specificity

Measures how narrow and discriminative a tag is.

- Interpretation:
  Higher means the tag points at a narrower semantic area.
- Expected range:
  `0..1`
- Likely inputs:
  - semantic judgment
  - embedding neighborhood density or rarity
  - optional catalog-relative context
- Rules:
  - persisted specificity must be deterministic
  - specificity is not the same as popularity
  - broad tags can still be highly useful; they are just less discriminative

### Content Confidence

Measures how much signal exists in the issue text.

This narrower v1 definition now lives in [content-confidence-spec.md](./content-confidence-spec.md).

- Interpretation:
  Higher means the issue text is rich enough to support more trustworthy tags, embeddings, and retrieval.
- Expected range:
  `0..1`
- Likely inputs:
  - token count
  - structural richness
  - amount of non-boilerplate text
- Rules:
  - text length should affect confidence, not importance
  - use saturating transforms such as `log(1 + x)`
  - very long text should not keep increasing confidence without bound
  - prefer search as the first validation surface before using confidence in map or PCA weighting

### Maturity

Measures how curated and developed an issue is.

- Interpretation:
  Higher means the issue has been refined into a more complete and useful representation.
- Expected range:
  `0..1`
- Likely inputs:
  - refinement count
  - progress history
  - amount of structured follow-up
- Rules:
  - refinement count alone is not maturity
  - maturity should rise when the issue becomes more complete, not merely more active

### Stability / Churn

Measures how settled an issue is.

- Interpretation:
  Higher churn means the issue's meaning is still moving; higher stability means it has converged.
- Expected range:
  `0..1`
- Likely inputs:
  - canonical text delta across refinements
  - tag profile delta across refinements
  - recent drift weighting
- Rules:
  - churn and maturity are related but not interchangeable
  - an issue can be mature and still unstable if it keeps being redefined

### Velocity

Measures recent rate of meaningful activity.

- Interpretation:
  Higher means the issue is receiving substantial updates in a short window.
- Expected range:
  `0..1`
- Likely inputs:
  - recent progress posts
  - recent refinements
  - distinct update events within a rolling window
- Rules:
  - velocity is not freshness
  - velocity is not quality

### Authority / Canonicality

Measures whether an issue acts as the authoritative landing point for a problem.

- Interpretation:
  Higher means other issues, merges, or links point to this issue as the canonical representation.
- Expected range:
  `0..1`
- Likely inputs:
  - inbound duplicate or merge links
  - reference patterns
  - long-lived resolution history
- Rules:
  - high connectivity alone is not authority
  - authority should be modeled separately from raw graph centrality

### Hubness

Measures graph centrality or connectivity in the issue-link graph.

- Interpretation:
  Higher means the issue sits near many other issues in the explicit relationship graph.
- Expected range:
  `0..1`
- Likely inputs:
  - graph degree
  - weighted centrality
  - relationship types
- Rules:
  - hubness is graph structure, not issue quality
  - noisy or generic issues can be hubs without being canonical

## Contextual Modifiers

These signals matter, but their meaning depends on the surface.

### Freshness / Time Decay

Time should not be treated as one global multiplier.

- For "what is active now," recency should boost.
- For "what still needs attention," long-open age may deserve a capped boost.
- For canonical closed issues, age may be neutral or even mildly positive if the issue remains referenced.

Recommended pattern:

- represent age-derived signals separately, such as:
  - `freshness`
  - `staleness`
  - `recent_activity`
- choose per-surface formulas instead of one repo-wide decay

### Resolution-Aware Weighting

Closed issues should not all behave the same.

- `fixed`, `duplicate`, `by_design`, `wont_fix`, and `stale` carry different retrieval and recommendation semantics
- closed state should usually act as a modifier, not as a rewrite of intrinsic issue quality

## Transform Rules

The following rules should apply across the model:

- Bound scores to a small stable range, usually `0..1`
- Use monotone transforms with saturation
- Prefer `log(1 + x)`, capped linear ramps, ratios, and half-life style decays
- Avoid direct use of raw counts in ranking formulas
- Avoid unbounded additive stacking of unrelated heuristics
- Name every blended formula and its inputs explicitly
- If a score is persisted, add fixture-based tests that prove it is reproducible

## Consumer Surfaces

### Search

Primary goal: retrieve the best matching issues for a query.

- Core inputs:
  - text retrieval score such as BM25
  - semantic similarity
  - content confidence
  - authority/canonicality
- Optional modifiers:
  - freshness
  - resolution-aware weighting
- Notes:
  - search should be the first major consumer used to validate the scoring model

### Explore / Related Issues

Primary goal: show meaningful adjacency around a known issue.

- Core inputs:
  - similarity
  - specificity-aware tag structure
  - authority
- Optional modifiers:
  - maturity
  - stability
- Notes:
  - explainability matters more here than raw retrieval recall

### Person Profiles and Recommendations

Primary goal: model depth, breadth, and likely next work.

- Core inputs:
  - specificity-weighted tag history
  - maturity
  - authority
- Optional modifiers:
  - freshness
  - velocity
- Notes:
  - "best next issue" is not the same objective as "most similar prior work"

### Map Weighting and Layout

Primary goal: produce stable, interpretable spatial structure.

- Core inputs:
  - factor/tag relevance structure
  - deterministic tag specificity where needed
- Optional modifiers:
  - confidence
  - maturity
  - stability
- Notes:
  - projection is a downstream consumer and should not define the scoring model

### Tag Quality and Tag UI

Primary goal: help users distinguish broad from discriminative tags.

- Core inputs:
  - specificity
  - tag-tag similarity
- Notes:
  - tag surfaces should explain specificity rather than treating it as magic

## Evaluation

Each new signal should come with:

- a short interpretation sentence
- a formula family or prototype definition
- test fixtures covering edge cases
- at least one known consumer
- at least one failure mode it is meant to reduce

Major consumer blends should be validated with offline examples before broad rollout. Search is the best first validation surface because quality changes are easiest to inspect there.

## Recommended Sequencing

1. Keep this scoring spec current as the architectural source of truth.
2. Fix deterministic tag specificity first.
3. Define and implement issue-quality primitives together:
   - content confidence
   - maturity
   - stability/churn
   - velocity
4. Add contextual modifiers:
   - time decay / freshness
   - resolution-aware weighting
5. Validate the model in search:
   - BM25-style text matching
   - explicit feature blending
6. Add graph structure signals:
   - hubness
   - authority/canonicality
7. Apply trusted signals to downstream visualization consumers:
   - weighted PCA covariance
   - frontend projection robustness

## Related Issues

- `01KKSVGDA4VBVDHVGXVG7AZSKW` scoring fundamentals roadmap
- `01KKSVH8314ND9NRS55GESR0FD` deterministic tag specificity
- `01KKSV70G7AD66QM0P3XJWG93T` content confidence
- `01KKSV7CY6P4CSXRDJCA4K0E2A` maturity
- `01KKSVH85D4XCRXGX6JT3CPSNA` stability/churn
- `01KKSV838DCAX6BD74K21RAZ8G` velocity
- `01KKSV6SEEEY8H3KZ4AG34WWGZ` time decay
- `01KKSV7R65DX5ANEARDFNS2H6Q` resolution-aware weighting
- `01KKSV7XT2TG46J53VF3H8NKA0` hubness
- `01KKSVH85CAFND1BA49HMY7Z85` authority/canonicality
- `01KKSV76MK9NFDKZ4TE5STBDA9` BM25-style text matching
- `01KKSV7JFJ2DQV6CWBRWMRTDV7` weighted PCA covariance
- `01KKSVH85CHWP867JZ1QVS6Q7W` frontend tag-map projection robustness
