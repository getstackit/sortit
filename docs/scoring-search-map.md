# Scoring, Search, and Map

Sortit uses two complementary representations:

- AI-inferred tag relevance scores for interpretable structure.
- Text embeddings for semantic similarity.

Most product surfaces blend both instead of treating either one as the whole truth.

## Tag Relevance

Each issue stores tag scores as continuous relevance values. An issue can be highly relevant to several tags at once, for example:

```json
[
  { "tag": "bug", "relevance": 0.8 },
  { "tag": "ui", "relevance": 0.3 },
  { "tag": "performance", "relevance": 0.1 }
]
```

Tag scores may also carry provenance and verifier metadata, including whether the tag was suggested, which candidate source selected it, alignment with the issue embedding, specificity, evidence ranges, verification verdict, and domination by a nearby unassigned tag.

Display tags are selected from explicit tags or from the highest ranked tag scores. When specificity is available, display ranking blends relevance and specificity so cards emphasize more discriminative tags.

## Tag Candidate Selection

Enrichment builds a retrieval-first candidate taxonomy:

1. Embed the issue text.
2. Retrieve nearby stored tag embeddings by cosine similarity.
3. Add explicit user tags.
4. Add a stable anchor set for broad recall.
5. Ask the analyzer to score that candidate set.

The system can still run full-catalog and explicit-only modes for debugging and evaluation, but the normal mutation path uses the retrieval shortlist.

## Tag Specificity

Specificity is a deterministic `0..1` score for how discriminative a tag is in the catalog.

The canonical implementation computes embedding-neighborhood specificity:

1. Use tags with embeddings.
2. For each tag, compute cosine distance to other tags.
3. Average the nearest-neighbor distances.
4. Convert those local distances to catalog-relative percentile ranks.

Low specificity means the tag lives in a dense or broad semantic area. High specificity means it is more semantically narrow. Missing specificity is treated as neutral by consumers.

Current consumers include:

- display tag ranking
- generic tag attenuation during enrichment
- search penalties for issues whose top tags are broad
- generic-query co-occurrence boosts when broad queries match issues with more specific tags
- tag detail and tag map UI
- person profile weighting

## Verification

The verifier is deterministic and runs after AI tag scoring. It uses:

- tag embedding alignment with the issue embedding
- specificity
- candidate source, such as retrieval, anchor, explicit, or full catalog
- grounded source-text evidence returned by the analyzer
- nearby unassigned candidate tags that better explain the issue

The verifier can keep a tag, down-rank it, or flag it for debug/review metadata. Grounded evidence can rescue a tag with weak embedding alignment.

## Search Ranking

Issue search starts with candidates from stored issue data and computes a combined score from:

- semantic similarity between query and issue embeddings
- factor similarity from tag relevance structure
- freshness weighting
- velocity boost
- authority boost from inbound canonical links
- specificity penalties and co-occurrence boosts
- content confidence as a near-tie breaker

Default blend constants live in `internal/scoring/constants.go`. The standard semantic/factor blend is `0.6 / 0.4`; when a query matches a known tag name, the factor side receives a small nudge.

Resolution state affects visibility and ranking through status filters and relationship semantics. Duplicate and merged issues can be hidden behind their canonical target depending on the surface.

## Content Confidence

Content confidence is a deterministic `0..1` score measuring how much useful signal exists in the canonical issue text. It is not priority or importance.

The scorer uses:

- token count with saturation
- content-token diversity
- structural signals such as multiple sentences, multiple lines, lists, paths, versions, and error-like text
- repetition penalties

Search uses content confidence to break near ties, so a richer issue can outrank a vague one when relevance is otherwise close.

## Freshness, Velocity, Maturity, Authority, and Hubness

Sortit keeps scoring primitives separate from consumer blends:

- Freshness is a bounded time weight with a floor and a 90-day half-life.
- Velocity measures meaningful recent activity from refinements, progress posts, and links over a 30-day window.
- Maturity estimates how developed an issue is, using content confidence and history.
- Authority measures inbound `duplicate_of` and `merged_into` links to identify canonical landing points.
- Hubness measures general graph connectivity separately from authority.

Search, explore, people recommendations, and map projection use different blends of these signals.

## Map Projection

The map is built from tag relevance, tag covariance, and PCA.

1. Build an issue-by-tag relevance matrix.
2. Build a tag covariance matrix from tag embedding cosine similarities.
3. Shrink off-diagonal covariance toward identity for stability.
4. Transform issue loadings by tag covariance, so related tags influence each other.
5. Weight issues by content confidence and maturity.
6. Run PCA and project to two dimensions.
7. Normalize positions robustly with outlier clipping.

The projection requires enough data to be meaningful: at least five issues and at least two tag dimensions.

Edges on the map come from embedding similarity. That makes position and edges complementary: position is tag/factor structure, while edges are semantic text similarity.

## Why Not Embeddings Only

Embeddings are strong at semantic retrieval but weak at explanation. The tag-factor model gives Sortit an interpretable structure: related issues can be explained by shared tags, factor similarity, and specific dimensions. Embeddings fill gaps where tags miss a relationship.
