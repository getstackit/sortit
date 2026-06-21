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

Tag scores may also carry provenance and verifier metadata, including whether the tag was suggested, which candidate source selected it, alignment with the issue embedding, specificity, evidence ranges, verification verdict, and domination by a nearby unassigned tag. Tags can additionally carry a negation score (an explicit "this tag does not apply" signal with its own provenance and evidence).

Display tags are selected from explicit tags or from the highest ranked tag scores. When specificity is available, display ranking blends relevance and specificity so cards emphasize more discriminative tags.

Note that tag-region membership (`internal/regions` `BelongsTo`) currently considers only positive relevance: an issue with relevance 0.5 and negation 0.7 — net signed relevance −0.2 — still counts as a region member. This is intentional for now (relevance and negation are treated as independent signals), but it is a candidate for switching to net-relevance membership (`relevance − negation ≥ floor`) once the math layer consumes signed scores.

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

The verifier can keep a tag, down-rank it, or flag it for debug/review metadata. Grounded evidence can rescue a tag with weak embedding alignment — but **not** a tag that is anti-aligned in centered space, which is suppressed regardless (see [Tag Taxonomy Health](#tag-taxonomy-health)).

## Tag Taxonomy Health

A tag taxonomy that skews generic is the single biggest threat to scoring quality. Left to bootstrap on its own, issue tagging drifts toward broad, cross-project tags (`backend`, `improvement`, `ui`) and away from the project-specific nouns that actually distinguish one issue from another. The symptom is a low **aggregate R²** (`sortit debug factor-weights`) — the fraction of corpus embedding variance the tag structure explains. An R² near 0.04 means the tags carry almost no signal, and the factor model responds by putting nearly all of its weight on the raw-text residual.

### Why generic tags win by default

Candidate selection (above) is retrieval-first: it scores the issue against tags already in the catalog plus a generic anchor set. A project-specific tag can only become a candidate once it already exists with an embedding — so new specific tags never bootstrap, the analyzer expresses every issue with the generic tags it can see, and the equilibrium locks in. See [onboarding.md](./onboarding.md) for the cold-start version of this problem and how to break it.

### Three levers that fix it

1. **Seed the vocabulary with concepts.** A [concept memory](./data-model.md#memory-and-curation-state) is the canonical profile of a noun, bound 1:1 to a tag. Authoring a concept registers its subject tag in the catalog (embedded from the concept body), so the curated noun becomes a retrieval candidate. Concepts are how a project teaches Sortit its own language.

2. **Gate concept synthesis by specificity.** Synthesized concept proposals are only drafted for tags above a specificity floor, so generic bucket tags — frequent but not meaningful nouns — never become concepts.

3. **Suppress anti-aligned tags, in centered space.** The verifier negates a tag whose embedding points *away* from the issue. The crucial subtlety: this is measured in the **centered** embedding space (the corpus-mean centering transform; see [whitepaper.md](./whitepaper.md)), not raw cosine. Raw embeddings are anisotropic — every tag-issue cosine is positive — so a raw-space check never fires. Only after subtracting the corpus mean does the drag appear as a negative cosine (e.g. `backend` at −0.10 on a ridge-regression issue). Anti-alignment overrides grounded evidence, because the generic-mention case (the word appears but the meaning points elsewhere) is exactly the target: in practice ~100% of generic-tag assignments carry lexical evidence.

### Measuring

`sortit debug factor-weights` gives the aggregate picture; `sortit debug issue-r2 <id>` shows a single issue's residual and the catalog tags nearest to it — the nouns it is about but isn't tagged with. Tagging quality (do issues get the right specific tags?) is the leading indicator and responds immediately to seeding; aggregate R² is the lagging, breadth-bound indicator that climbs as coverage and cleanliness improve. A sparse tag set has a real ceiling on how much of a high-dimensional text embedding it can reconstruct, so R² is best read as a relative health signal, not an absolute target.

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
