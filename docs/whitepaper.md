# Sortit: A White Paper on the Math Behind Sorting Freeform Issues

> Scope: this paper documents the mathematical model that underpins Sortit's
> enrichment, search, exploration, and map surfaces. It is also intentionally
> critical: where the implementation diverges from textbook treatments, or where
> a choice is more heuristic than principled, that is called out.
>
> It complements the [Scoring, Search, and Map](./scoring-search-map.md) doc,
> which describes the surface behavior, by digging into why the math is shaped
> the way it is and where it bends.

---

## 1. Problem framing

Sortit ingests freeform text — a bug report, a customer quote, a feature note,
a stack trace — and needs to make that text:

1. **Findable** by humans typing imprecise queries.
2. **Comparable** to other issues so duplicates and adjacencies surface.
3. **Explainable**: a user should be able to see *why* two issues are linked.
4. **Layoutable** on a 2-D map that groups related issues without losing
   semantic detail.

Embeddings alone solve (1) and (2) but not (3). A pure tag taxonomy solves (3)
and (4) cleanly but is brittle when tags are sparse or ambiguous. Sortit
therefore maintains **two parallel representations of every issue**:

- A text embedding `e ∈ ℝ^D` produced by the AI analyzer.
- A tag relevance vector `r ∈ [0,1]^T` over an evolving catalog of `T` tags.

The math is the glue that turns these two representations into a single,
explainable ranking signal.

---

## 2. The factor / residual decomposition

> **Status (Phase 3c):** the similarity-ranked surfaces — issue **search**,
> **explore** (related issues), and **person recommendations** — now default
> to the full-rank anchored-ridge model described in
> [math-evolution.md §5](./math-evolution.md#5-anchored-ridge-regression-f_i),
> with its unscored-tag penalty selected per corpus by GCV
> (`internal/ridgelambda`). The rank-1 decomposition documented in this
> section remains the **fallback** on those surfaces (small corpora, or no
> penalty available) and still backs the debug R²/factor-weight endpoints.
> The 2-D **map projection** is unaffected — it is PCA on the tag-relevance
> matrix (§3), not this embedding decomposition. The rank-1 model is
> documented here because it is the foundation the ridge model refines and
> the fallback every ridge surface degrades to.

The central trick in Sortit is to split each issue's embedding into:

- A **factor** component — the part of the embedding that can be predicted
  from the issue's tags.
- A **residual** component — what's left over, the embedding-only signal.

### 2.0 Anisotropy and corpus-mean centering

Everything in this section operates on **corpus-mean centered** embeddings,
not the raw vectors the embedding service returns. The reason is a
well-known property of text embedding models (including
`text-embedding-3-small`): over a topically homogeneous corpus — and a
Sortit workspace is, by construction, all software-issue text — every
vector shares a large common direction. Raw cosines therefore cluster in a
narrow, high band: ~0.7 between two *unrelated* issues is typical. That
shared component is anisotropy, not signal, and before centering it
inflated every quantity built from these cosines: the tag covariance Σ
(§2.1), per-issue `R²` and the factor weight `w_F` (§2.3–2.4), and the
factor similarities used in search ranking (§7).

The fix (in the spirit of "All-but-the-Top", Mu & Viswanath 2018) is
applied at the corpus-load boundary, before any of the math below runs:

```
e_centered = unit(e − mean(issue corpus))
t_centered = unit(t − mean(tag catalog))
```

Issue embeddings are centered against the issue-corpus mean and tag
embeddings against the tag-catalog mean, then re-normalized
(`internal/issuemath/centering.go`). Details that matter:

- **Centering is a runtime transform.** Persisted embeddings are never
  rewritten. The means are recomputed from the store and cached keyed by
  corpus revision (`internal/centering`), the same mechanics as the tag
  co-occurrence cache.
- **External vectors are centered with the stored corpus means, never
  their own.** A search query embedding (and a person embedding) is
  centered with the cached issue-corpus mean. This is load-bearing on the
  semantic-search path: the candidate set is a similarity-retrieved
  subset, and that subset's own mean points toward the query
  neighborhood — subtracting it would remove exactly the signal being
  searched for.
- **Degenerate corpora skip centering.** Populations smaller than
  `MinCenteringVectors` keep their raw (unit-normalized) vectors — a mean
  over a handful of vectors is noise, and subtracting it makes
  near-parallel vectors antipodal. A vector that becomes zero after
  centering (e.g. a single-vector corpus, where the mean is the vector
  itself) falls back to its uncentered form.
- **Aggregate `R²` dropped when centering landed, and that is correct.**
  Pre-centering `R²` counted the shared anisotropic direction as "variance
  explained by tags," because both the issue embedding and the synthesized
  factor direction contained it. Post-centering numbers are smaller and
  honest; the matheval baseline records the drop, alongside an improvement
  in ranking quality (NDCG@8 0.823 → 0.874, Recall@8 0.855 → 0.927 on the
  fixture corpus).
- **Map edges are the one deliberate exception** — see §3.2.

### 2.1 Tag covariance Σ

Each tag in the catalog has its own embedding (the analyzer embedding of
`"<name> - <description>"`). The system builds a `T × T` similarity matrix:

```
Σ[i,j] = cos(tag_i.embedding, tag_j.embedding)
```

with the diagonal set to 1.

Σ is then **shrunk toward the identity** (see [`projection.go:235`](../internal/issuemath/projection.go) `buildTagCovariance`):

```
Σ_shrunk = α · Σ + (1 − α) · I
α = clamp(1 − mean(off-diag²), 0.1, 1.0)
```

This is the system's regularizer for a noisy or small tag catalog. As tags
become more correlated, α drops and Σ is dragged toward independence.

### 2.2 Synthesized factor direction

For a given issue with relevance vector `r ∈ ℝ^T` and a tag-embedding matrix
`T ∈ ℝ^(T×D)` whose rows are the per-tag embeddings, the factor-predicted
direction is:

```
ê = Tᵀ (Σ_shrunk · r)
```

That is: smear the tag relevances through the tag similarity matrix, then
lift the result into embedding space by linear combination of tag embeddings.
The covariance multiplication is what makes `ê` continuous: an issue tagged
only with `crash` ends up with non-zero loading on `bug`, `safari`, etc., if
those tags are semantically nearby.

### 2.3 Orthogonal split

Given the issue's actual embedding `e`, project onto the normalized direction
`û = ê / ||ê||`:

```
factor   = (e · û) û     if e · û > 0, else 0
residual = e − factor          (orthogonal to factor by construction)
```

The projection is **non-negative**: when the centered embedding is
anti-aligned with its own tag direction (`e · û ≤ 0`), the issue is treated
as having *no factor evidence* — zero factor, full-embedding residual,
`R² = 0` — rather than projected onto `−û`. A sign-flipped factor would
score ≈ −1 against issues sharing the same tags, asserting strong
dissimilarity where the honest claim is "the text does not support the
tagging." Tags assert non-negative alignment; the projection respects that.

Because the split is orthogonal:

```
||e||² = ||factor||² + ||residual||²
R²_i   = 1 − ||residual||² / ||e||²    ∈ [0, 1]
```

The per-issue `R²` is exposed at `GET /api/v1/debug/issues/{id}/r2` and serves
as the system's measure of *how well the tag-factor story explains the
embedding text*.

### 2.4 Data-driven blend weights

After processing all issues, the system aggregates explained vs. residual
variance and derives the blend weights for the issue corpus:

```
w_F = clamp(σ²_factor / (σ²_factor + σ²_residual), 0.05, 0.95)
w_R = 1 − w_F
```

The aggregate `R²` reported on `/debug/factor-weights` is the same ratio.
Floors at 0.05 and 0.95 ensure that neither signal is ever fully ignored —
even when the model is doing badly globally, there is still some weight on
both sides.

The blended similarity used for ranking is then:

```
sim(A, B) = w_F · cos(factor_A, factor_B) + w_R · cos(residual_A, residual_B)
```

**Degenerate pairs marginalize to the evidence that exists.** When either
side of a pair has a zero factor (untagged, or anti-aligned per §2.3), the
blend puts full weight on the residual signal — and symmetrically, full
weight on the factor side when a residual is zero. Without this rule such
pairs would top out at `w_R` (or `w_F`) in the same ranked list as
full-scale pairs: an untagged search query would have *every* candidate
deflated by `w_F`, letting the fixed-size additive boosts (§7) and the
0.05 tie window dominate the deflated similarity scale. Candidates missing
from the decomposition entirely (no persisted embedding, or a dimension
mismatch after an embedding-model change) are scored as pure semantic
similarity at full weight for the same reason.

### 2.5 What this is, and what it isn't — a critical note

The decomposition is presented as a "factor model" but mathematically it is
a **rank-1, per-issue projection** of the embedding onto a single
issue-specific direction. It is not a multivariate factor analysis, not a
PCA on issue embeddings, and not a kernel-style decomposition.

Implications:

- **Per-issue `R²` is upper-bounded by the squared cosine between `e` and `ê`.**
  Even a perfectly tagged issue has low `R²` whenever the embedding model
  doesn't agree with the linear combination of tag embeddings. This is fine
  as a diagnostic but does *not* mean tagging is poor.
- **Both similarity sides compare unit directions; magnitudes do not
  influence ranking.** This is now a *measured* decision rather than an
  accident: scaling residual similarity by the residual magnitudes (the
  unnormalized residual dot product) dropped fixture NDCG@8 by 0.033, and a
  geometric-mean variant was a wash. The residual direction of a
  well-explained issue is its discriminating content within a topic
  cluster, not amplified noise. The magnitudes remain available on
  `DecomposedEmbedding` for diagnostics.
- **The aggregate weights `w_F, w_R` are corpus-wide, not query-aware.**
  Searches that should weight factors more heavily (e.g. a tag-name query)
  get a fixed `+0.1` nudge instead of a recomputation. That nudge is itself
  a hyperparameter. The identification `w_F = aggregate R²` is, however,
  no longer unvalidated: the matheval sweep (`-sweep`) shows fixture
  NDCG@8 plateaus over `w_F ∈ [0.55, 0.90]` and the data-driven weight
  plus nudge sits in that plateau, slightly above the best fixed override.
- The name is aspirational. A more honest label would be
  *embedding-to-tag-direction projection* — but the implementation does in
  fact behave like a soft factor model in practice.

---

## 3. Map projection

The 2-D map is *not* built from embeddings directly. It is built from the
issue-by-tag relevance matrix transformed through tag covariance, then
PCA'd.

Let:

- `X ∈ ℝ^(N×T)` — relevance matrix
- `Σ_shrunk` — same tag covariance as above
- `w ∈ ℝ^N` — per-issue quality weights (see below)

The pipeline ([`projection.go:23`](../internal/issuemath/projection.go) `ComputePositions`):

1. `X' = X · Σ_shrunk` — smear loadings across correlated tags.
2. Compute weighted column means and mean-center `X'`.
3. Row-scale by `√w` so that `X'ᵀ X' / (Σw − 1)` is a properly weighted
   sample covariance `C ∈ ℝ^(T×T)`.
4. Eigendecompose `C` (symmetric) and take the top 2 eigenvectors `V₂`.
5. Project: `P = X' · V₂ ∈ ℝ^(N×2)`.
6. Robustly normalize each axis to `[0.05, 0.95]` using IQR clipping
   (`[Q1 − 1.5·IQR, Q3 + 1.5·IQR]`).

### 3.1 Per-issue projection weights

```
w_i = max(0.1, contentConfidence(raw_i) · maturity_i)
```

Both factors are in `[0, 1]`. Floor of 0.1 prevents any issue from being
completely ignored. The point is that high-confidence, well-developed issues
should dominate the principal axes; low-signal issues should ride along.

### 3.2 Edges

Edges on the map are **not** derived from the projected positions. They are
computed directly from issue embeddings:

```
edge(i, j) exists iff cos(e_i, e_j) ≥ threshold
```

This makes position (tag-factor structure) and edges (raw semantic
similarity) **complementary**. Two issues can be far apart in the layout
because they have different tags but still be connected by a strong
edge if their language is similar.

Edges are also the one consumer that deliberately keeps the **uncentered**
embeddings (see §2.0): the `minEdgeSimilarity` threshold and the UI's edge
density were tuned against the raw cosine scale, and centering collapses
that scale (typical same-corpus cosines drop from ~0.7–0.9 to ~0–0.3),
which would silently disconnect most of the map. The eval harness in
`internal/matheval` does not cover edge quality, so there was no evidence
to justify re-tuning the threshold; if edges are ever moved to centered
similarities, `minEdgeSimilarity` must be re-derived at the same time.

### 3.3 Critical notes on the projection

- **`X' = X·Σ` is still in tag-relevance space.** PCA on `X'` finds the two
  dominant axes of *smeared tag loadings*, not the principal directions of
  issue embeddings. As Σ → I (which the shrinkage drives in poorly-correlated
  catalogs), this reduces to PCA on plain `X`.
- **Orientation is stabilized in two layers.** Eigenvector signs are
  arbitrary, and when the top two eigenvalues are close, PC1/PC2 can swap
  between recomputes. First, a deterministic loading-sign convention is
  applied: the tag with the largest absolute loading on each principal axis
  must load positively, making orientation a property of the data rather
  than input order (an earlier version flipped axes based on whichever
  issue happened to be first in the input). Second, when a previous layout
  exists with ≥ 3 shared issues, the raw projected coordinates are aligned
  to it by orthogonal Procrustes: `Q = UVᵀ` from the SVD of the 2×2
  cross-covariance of the centered shared points, with reflections allowed
  so both sign flips and component swaps are corrected. The reference is
  the previous *normalized* layout (what users saw); only the orthogonal
  part is kept, since the per-axis robust normalization re-establishes
  translation and scale. The previous layout is held in an in-memory
  last-layout cache on the projection loader and seeded from persisted
  projections on load.
- **Robust normalization is conditionally robust.** If the IQR collapses to
  zero, the function falls back to min-max — which is *not* robust. With
  highly clustered loadings (most issues sharing the same two tags) this
  fallback can fire.
- **The shrinkage rule is heuristic.** Classical Ledoit-Wolf shrinkage
  computes α from sample size and dimension and has well-understood
  asymptotic guarantees. Sortit's `α = 1 − mean(off-diag²)` is the wrong
  direction in spirit: it shrinks more when the off-diagonal *carries
  signal* (high pairwise similarity). It happens to work because the goal
  isn't optimal covariance estimation but PCA stabilization. Worth keeping,
  worth documenting honestly.

---

## 4. Tag specificity

Specificity is the system's deterministic measure of how *discriminative* a
tag is in the catalog ([`specificity.go`](../internal/issuemath/specificity.go) `NeighborhoodSpecificity`).

For each tag with an embedding:

1. Compute cosine distance to every other tag.
2. Take the mean distance over the `k = min(8, n−1)` nearest neighbors.
3. Percentile-rank these means across the catalog.

Output `s ∈ [0, 1]`. A `bug`-like tag living in a dense, generic neighborhood
gets a low score. A `safari-share-sheet`-like tag in a sparse neighborhood
gets a high score.

### 4.1 Where specificity is consumed

- **Display ranking**: blend relevance with specificity so cards show
  discriminative tags first.
- **Generic tag attenuation during enrichment** ([`verify.go:33`](../internal/issueenrichment/verify.go)): if an issue has at least one
  specific tag, the relevance of any generic tag (`s < 0.3`) is multiplied
  by 0.6.
- **Search penalty**: each generic tag in an issue's top-3 contributes
  `(1 − s) · 0.04` of penalty, averaged.
- **Co-occurrence boost**: if the query itself names a generic tag, issues
  carrying *specific* tags alongside the generic one get a small boost
  (per specific tag: `0.03`, cap `0.06`).
- **Person profiles**: tag relevance is weighted by specificity when
  aggregating a person's profile, with missing specificity defaulting to
  `0.5` (the generic threshold), not `1.0`.

### 4.2 Critical notes on specificity

- **`k = 8` is hard-coded.** With a small catalog this collapses to all-pairs;
  with a large catalog it captures only local density. The choice of 8 is
  unjustified and would benefit from being made adaptive (e.g., `k = √n`).
- **Percentile ranks are catalog-relative**, which means *adding tags
  shifts every tag's specificity*. This is mostly benign because
  consumers treat the score as relative anyway, but it does mean that any
  cached specificity goes stale on every catalog change.
- **The fallback for missing specificity is `0.5`**, the same as the
  `GenericTagThreshold`. That means unscored tags are treated as
  borderline-generic in every consumer. This is a hidden prior worth
  surfacing.

---

## 5. Content confidence

A deterministic `[0, 1]` measure of how much usable signal lives in an
issue's text ([`content_confidence.go:25`](../internal/issueanalytics/content_confidence.go)):

```
C = clamp01(
    0.25
  + 0.45 · L     length saturation: 1 − exp(−tokens/80)
  + 0.15 · D     diversity ramp on uniqueRatio
  + 0.20 · S     structure signals matched / 4
  − 0.25 · R     repetition penalty
)
```

Where `S` counts up to 4 of {≥2 sentences, ≥2 lines, list markers, technical
detail (errors, paths, versions)} and `R = max(duplicateLineRatio,
lowDiversityFlag)`.

### 5.1 Where content confidence is consumed

- **Search tie-breaking**: when two issues are within
  `ContentConfidenceTieWindow = 0.05` of each other, the higher-confidence
  one wins.
- **PCA projection weighting**: directly multiplies maturity to form `w_i`.
- **Maturity**: feeds into the maturity formula at weight 0.20.

### 5.2 Critical notes

- **Coefficients (0.45 / 0.15 / 0.20 / −0.25 around a 0.25 base) are
  hand-tuned**, not learned. There is no held-out validation that the
  resulting score matches a human's judgment of "how informative is this
  text."
- **The diversity ramp** `(uniqueRatio − 0.25) / 0.45` is reasonable but
  the inflection points are unmotivated.
- **Short-text attenuation kicks in below 12 content tokens**, with a linear
  ramp from 4–12 tokens. Issues shorter than 4 content tokens get a
  diversity contribution of zero. The 4/12 boundary is unjustified.

The score is *useful as a tie-breaker* — that's its main use — and the
calibration there is less critical than for a primary ranking signal.

---

## 6. Lifecycle primitives: freshness, velocity, maturity, authority, hubness

These are intentionally separate scoring primitives. Each is `[0, 1]`. The
consumers combine them with different weights for different products.

### 6.1 Freshness

```
freshness(t) = floor + (1 − floor) · exp(−ln(2) · ageDays / halfLifeDays)
```

with `floor = 0.3`, `halfLife = 90d`. Age is computed over the latest of
created/closed/post/link timestamps.

### 6.2 Velocity

A windowed, half-life-decayed weighted count of recent meaningful events:

```
events    = { refinement (1.0), progress (0.85), link (0.65) } within last 30d
weighted  = Σ wᵢ · exp(−ln(2) · ageDaysᵢ / 14)
velocity  = clamp01(1 − exp(−weighted / 2.5))
```

The saturation curve means a handful of recent events is enough to push
velocity near 1.

### 6.3 Maturity

```
activity   = 0.75 · (1 − exp(−refinements/2)) + 0.25 · (1 − exp(−progress/2))
maturity   = clamp01(0.10 + 0.35·activity + 0.20·contentConfidence + 0.35·stability)
```

with `stability` defaulting to 0.35 when the lifecycle metric is absent.

### 6.4 Authority and hubness

```
authority = min(1, dupCount · 0.25)        // inbound duplicate_of + merged_into
hubness   = min(1, linkCount · 0.15)        // all inbound + outbound links
```

These are explicitly **separate signals**: authority asks "is this the
canonical landing point for a problem?", hubness asks "is this a
graph-central anchor?". Search and explore use authority; map UI uses
hubness; cluster top-tag and label both use hubness as a per-issue weight.

### 6.5 Critical notes on the primitives

- **Velocity decay is now windowless.** The decayed weight has no cutoff —
  the 14-day half-life drives old events toward zero smoothly (an event at
  60 days carries ~0.05 of its weight) instead of stepping to zero at day
  30. The 30-day window survives only in `RecentActivityCount`, which
  answers a different question ("how many things happened lately?").
- **Authority and hubness use different slopes (0.25 vs 0.15)** for no
  documented reason. The asymmetry seems incidental rather than considered.
- **Maturity's stability fallback (0.35) is a hidden prior.** Without any
  lifecycle metric, an issue starts at `0.10 + 0.35·0.35 = 0.2225` from
  stability alone. That's the floor for "we know nothing."

---

## 7. How signals combine in search

[`search.go`](../internal/map/search.go) composes scores under one rule:
**query-relative evidence adds, candidate quality multiplies.**

```
evidence  =  blended_similarity                   // w_F·cos(F_q,F_i) + w_R·cos(R_q,R_i)
evidence +=  co-occurrence boost (generic query)  // additive, 0..0.06
evidence +=  region-match boost                   // additive, 0.08
evidence -=  anti-correlation penalty             // additive
evidence  =  max(evidence, 0)                     // no evidence → no score

combined  =  evidence
combined *=  freshness(i)                         // 0.3..1.0
combined *=  1 + 0.08 · velocity(i)               // 1.0..1.08
combined *=  1 + 0.10 · authority(i)              // 1.0..1.10
combined *=  1 − specificityPenalty(i.top3)       // 0.96..1.0
```

The additive terms all assert how well the candidate matches *this query*;
the multiplicative terms are query-independent properties of the candidate
and can only scale evidence — never flip its sign, resurrect a
zero-evidence candidate, or push the score outside a predictable range.
Under the previous mixed composition, additive authority could dominate the
sign of a negative blend, and the freshness multiplier made negative scores
*better* as issues aged. Explore and person recommendations follow the same
rule; in explore the human-declared relationship boost counts as evidence
(added after the clamp, so an explicit link surfaces even at zero embedding
similarity).

Tie-breaker (within `ContentConfidenceTieWindow = 0.05`): higher
`contentConfidence` wins; then raw `combined`; then `semantic`; then
`factor`; then `id` for determinism.

### 7.1 Critical notes on the combination

- **The score range is now non-negative and bounded** (evidence ≤ ~1.14
  before modifiers, modifiers ≤ ~1.19 combined), so the 0.05 tie window
  operates on a predictable scale. Candidates with negative blended
  similarity all clamp to zero and resolve by the tie-breaker chain.
- **Explore uses `freshness^0.5` instead of `freshness`** — now an explicit
  constant (`scoring.ExploreFreshnessExponent`) with a documented
  rationale: explore ranks the neighbors of a specific issue, where
  staleness matters less than relatedness, so the decay range is
  compressed rather than matched to search.
- **Tag-name correlation has two implementations.** The decomposition path
  nudges `w_F` by `+0.1`; the legacy fallback substitutes fixed `0.5/0.5`
  weights from `TagCorrelationSemantic/Factor`. The two paths produce
  different rankings on the same data depending on whether decomposition
  succeeded.
- **The velocity boost is small** (multiplier in `[1.0, 1.08]`) and the
  authority boost is also small (additive in `[0, 0.10]`). Their *relative*
  importance is dominated by `blended` for most queries. That's probably
  the right design, but the constants are not derived from any explicit
  utility model — they're tuned by feel.

---

## 8. The verifier

The verifier ([`verify.go:77`](../internal/issueenrichment/verify.go)
`decorateAndVerifyTagScores`) runs after the AI analyzer scores tags and
adds three things to each assigned tag:

1. **Embedding alignment**: `cos(issueEmbedding, tagEmbedding)`.
2. **Dominance check**: among *unassigned* candidate tags with embeddings,
   find any whose alignment exceeds the assigned tag's by `≥ 0.18` *and*
   whose absolute alignment is `≥ 0.35`. The best such candidate, if any,
   "dominates" the assigned tag.
3. **Grounded evidence**: source-text quote ranges returned by the
   analyzer.

A small state machine then decides whether to keep, flag, or down-rank:

| Condition                                                                 | Verdict     |
|---------------------------------------------------------------------------|-------------|
| Grounded evidence + very weak alignment (`< 0.08`) + high relevance       | **Keep**    |
| No evidence + very weak alignment + high relevance                        | **Flag**    |
| Grounded evidence + dominating candidate + weak alignment (`< 0.16`)      | **Keep**    |
| Dominating candidate + weak alignment                                     | **DownRank** (× 0.75, floor 0.08) |
| Anchor-only candidate + weak alignment + moderate relevance               | **Flag**    |
| Otherwise                                                                 | **Keep**    |

### 8.1 Critical notes on the verifier

- **The thresholds (0.08, 0.16, 0.18, 0.35) are unjustified.** They're
  reasonable choices for cosine similarities on OpenAI embeddings but no
  evaluation backs them up.
- **The verifier is a post-hoc rules engine, not a probabilistic model.** It
  is therefore predictable and auditable (a good property) but cannot
  improve from feedback.
- **Source-text grounded evidence is a strong rescue signal.** It is
  responsible for keeping otherwise-flagged tags when the analyzer can
  quote a concrete substring of the issue. The trust placed in the
  analyzer's ability to honestly cite the source text is real; there is
  no second-pass check that the quote actually appears (well — there is:
  `resolveEvidenceRanges` walks the normalized text and rejects quotes
  it can't find. So the evidence rule is at least cross-verified).

---

## 9. The embedding fallback

When the system needs an issue embedding but the persisted vector is
missing, [`runtime.go:376`](../internal/map/runtime.go)
`runtimeIssueEmbedding` synthesizes one:

```
v = 0.7 · embeddingFromText(raw)
  + Σ_tags 0.9 · relevance · tagEmbedding
v ← v / ||v||
```

`embeddingFromText` is a **hash-based** "embedding": each token is hashed
into the vector at three sign-flipped indices via FNV. This is a bag-of-words
sketch, not a learned embedding. Cosine similarity between two such vectors
fires only on token collisions and carries no semantic meaning beyond
overlap of surface tokens.

This is the system's emergency fallback path. It should never fire on a
healthy production install, but it does fire in tests and could fire in
cold-start scenarios. **The very existence of this fallback is a soft
correctness risk**: if persisted embeddings silently disappear for any
reason, search and map quality will degrade in a way that's hard to detect
because the system keeps producing results.

The fallback is therefore instrumented: each time a hash pseudo-embedding
stands in for a missing persisted embedding, an in-process counter is
incremented by kind (issue vs tag) and a rate-limited warning (once per
minute per kind) is logged. Cumulative counts are exposed via
`GET /api/v1/debug/embedding-fallbacks` — non-zero values on a production
install mean embeddings are silently missing and quality is degraded. The
fallback behavior itself is unchanged.

---

## 10. What is sound, what isn't, and what to fix

### 10.1 Solid foundations

- **Two-representation design** (embeddings + tags) is the right call for
  a system that has to both rank and explain.
- **Orthogonal factor/residual split** is mathematically clean and gives
  the data-driven blend weights a real justification.
- **Specificity via kNN percentile rank** is deterministic and stable
  enough to be cached.
- **Lifecycle primitives are appropriately separated** from their consumer
  blends; recombining them differently per surface is a clean architecture.
- **The verifier's grounded-evidence rule actually verifies the quote
  appears in the source.** That's the kind of "check the prompt didn't
  hallucinate" hygiene that's easy to skip.

### 10.2 Weak spots worth tightening

| # | Issue                                                          | Suggested direction                                          |
|---|----------------------------------------------------------------|--------------------------------------------------------------|
| 1 | Single-direction "factor model" is oversold                    | Addressed: the similarity surfaces (search/explore/person) now default to the full-rank anchored ridge `f = (TTᵀ+Λ)⁻¹(Te+Λr)` (math-evolution Phase 3c), a genuine multi-dimensional loading rather than a rank-1 projection. This rank-1 decomposition is now the documented fallback. |
| 2 | Shrinkage `α = 1 − mean(off-diag²)` is heuristic               | Replace with Ledoit-Wolf or document explicitly as a stability hack. |
| 3 | Map sign-convention depends on first issue                     | Addressed: deterministic largest-absolute-loading sign convention plus Procrustes alignment against the previous layout (§3.3). |
| 4 | Hash-based "embedding" fallback hides degradation              | Addressed: per-kind counters plus rate-limited warning logs, exposed via `/api/v1/debug/embedding-fallbacks` (§9). Making search return an error instead remains open. |
| 5 | Inconsistent recency exponents (`freshness` vs `√freshness`)   | Addressed: explore's softer exponent is a named constant (`ExploreFreshnessExponent`) with documented rationale (§7.1). |
| 6 | Mixed additive/multiplicative composition in `combined`        | Addressed: evidence adds and clamps at zero, quality modulates multiplicatively, across search/explore/person recommendations (§7). |
| 7 | Velocity hard window at 30 days                                | Addressed: decayed weight is windowless; the window survives only in `RecentActivityCount` (§6.5). |
| 8 | Authority and hubness slopes (0.25 vs 0.15) are asymmetric     | Pick from a deliberate utility argument, or unify them. |
| 9 | Hyperparameters everywhere with no evaluation harness          | Addressed: [`internal/matheval`](./math-eval.md) runs a labeled judgment set (32 queries over a 48-issue fixture corpus) against a golden baseline on every test run. |
| 10 | `k = 8` in specificity is fixed                                | Make adaptive (`min(8, ⌈√n⌉)` or similar). |
| 11 | Embedding anisotropy inflated every cosine-derived quantity   | Addressed: runtime corpus-mean centering at the corpus-load boundary (§2.0), with revision-cached means; map edges deliberately excluded (§3.2). |

### 10.3 Already landed from the evolution plan

This paper describes the *current* math; [math-evolution.md](./math-evolution.md)
describes where it is heading. Two pieces of that plan have since shipped
and are part of the current system:

- **Signed relevance (`r⁻`) is end-to-end.** The analyzer emits
  `negated_tags` with verbatim evidence quotes
  (`internal/ai/openai.go`), the verifier cross-checks the quotes against
  the source text before applying them
  (`internal/issueenrichment/verify.go` `applyAnalyzerNegations`), and the
  result persists as `Negation` / `NegationProvenance` /
  `NegationEvidence` on `TagRelevance`. Verifier dominance also emits a
  negation instead of only multiplying relevance down.
- **Anchored ridge regression exists in shadow mode.** The
  `f = (TTᵀ + Λ)⁻¹(Te + Λr)` solve with per-tag anchors
  (`internal/issuemath/ridgescore.go`) is computed on demand behind
  `GET /api/v1/debug/issues/{id}/ridge`, anchored on signed `r⁺ − r⁻`,
  along with a drift cosine between anchor and refined scores. It is not
  persisted and not consumed by ranking.

See math-evolution.md §10.1 for the full phase-by-phase status.

### 10.4 Things to leave alone

- The decision to keep edges on the map driven by raw embedding similarity
  while positions come from tag-factor PCA. This is a clean separation of
  concerns and produces a usable visualization.
- The verifier's bias toward grounded source-text evidence. This is
  difficult to over-tune wrong because the evidence is independently
  verified.
- The data-driven blend weights with floors. The floors prevent the system
  from going off the rails when the corpus is too small for stable
  variance estimates.
- Content confidence as a *tie-breaker only*. Promoting it to a primary
  signal would expose the hand-tuned coefficients to far more scrutiny
  than they can currently bear.

---

## 11. Closing read

Sortit's math is **honest heuristic work, dressed in factor-model
vocabulary**. The decomposition is a rank-1 projection that behaves
soft-factor-like in aggregate. The map projection is PCA on smeared tag
loadings, not on embeddings. The verifier is a hand-built rules engine that
nonetheless performs a real verification step. The lifecycle primitives are
saturated exponentials with hand-picked half-lives.

None of this is wrong; much of it is well-shaped for the size and shape of
data Sortit handles. The biggest risk is **drift from these defaults
without measurement**: any future tuning of constants in
`internal/scoring/constants.go` should be backed by an evaluation harness,
not by hand-feel, before it lands. That harness now exists: see
[Math Evaluation Harness](./math-eval.md) (`internal/matheval`), which runs
NDCG@8/Recall@8 over a labeled judgment set plus per-issue R² distribution
checks against a golden baseline as part of the normal test suite.

---

## Appendix A: Where the math lives

| Concept                                | File                                                                  |
|----------------------------------------|-----------------------------------------------------------------------|
| Tag covariance, shrinkage              | [`internal/issuemath/projection.go`](../internal/issuemath/projection.go) |
| Factor/residual decomposition          | [`internal/issuemath/factor_model.go`](../internal/issuemath/factor_model.go) |
| Map projection (PCA)                   | [`internal/issuemath/projection.go`](../internal/issuemath/projection.go) |
| Specificity (kNN percentile)           | [`internal/issuemath/specificity.go`](../internal/issuemath/specificity.go) |
| Content confidence                     | [`internal/issueanalytics/content_confidence.go`](../internal/issueanalytics/content_confidence.go) |
| Freshness                              | [`internal/issueanalytics/time_decay.go`](../internal/issueanalytics/time_decay.go) |
| Velocity                               | [`internal/issueanalytics/velocity.go`](../internal/issueanalytics/velocity.go) |
| Maturity                               | [`internal/issueanalytics/maturity.go`](../internal/issueanalytics/maturity.go) |
| Authority, hubness                     | [`internal/issueanalytics/authority.go`](../internal/issueanalytics/authority.go), [`internal/issueanalytics/hubness.go`](../internal/issueanalytics/hubness.go) |
| Search blend                           | [`internal/map/search.go`](../internal/map/search.go) |
| Explore blend                          | [`internal/map/explore.go`](../internal/map/explore.go) |
| Cluster k-means + silhouette           | [`internal/map/cluster.go`](../internal/map/cluster.go) |
| Verifier rules                         | [`internal/issueenrichment/verify.go`](../internal/issueenrichment/verify.go) |
| All tunable constants                  | [`internal/scoring/constants.go`](../internal/scoring/constants.go) |
| Cosine helpers                         | [`internal/vectors/cosine.go`](../internal/vectors/cosine.go) |
| Corpus-mean centering                  | [`internal/issuemath/centering.go`](../internal/issuemath/centering.go), [`internal/vectors/centering.go`](../internal/vectors/centering.go) |
| Revision-cached corpus means           | [`internal/centering/cache.go`](../internal/centering/cache.go) |

## Appendix B: Notation

| Symbol         | Meaning                                                       |
|----------------|---------------------------------------------------------------|
| `D`            | Embedding dimension (analyzer-defined)                        |
| `T`            | Active tag catalog size                                       |
| `N`            | Issue count                                                   |
| `e_i ∈ ℝ^D`    | Issue `i`'s text embedding (unit-normalized for cosine)       |
| `r_i ∈ [0,1]^T`| Issue `i`'s tag relevance vector                              |
| `T ∈ ℝ^(T×D)`  | Tag-embedding matrix, rows are tag embeddings                 |
| `Σ ∈ ℝ^(T×T)`  | Tag covariance: cosine of pairs of tag embeddings             |
| `Σ_shrunk`     | `α·Σ + (1−α)·I` shrinkage-regularized covariance              |
| `ê_i`          | `Tᵀ Σ_shrunk r_i`, the "factor direction" for issue `i`      |
| `factor_i`     | Projection of `e_i` onto `ê_i / ||ê_i||`                       |
| `residual_i`   | `e_i − factor_i`, orthogonal to `factor_i`                    |
| `R²_i`         | `1 − ||residual_i||² / ||e_i||²`                              |
| `w_F, w_R`     | Aggregate factor/residual blend weights, clamped to [0.05, 0.95] |
