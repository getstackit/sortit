# Sortit Math Evolution: A Quantitative Planning Layer

> Scope: this document is the ledger and roadmap for Sortit's math layer. It is
> **part manifest** (Part I: what shipped, as actually built, with the places the
> implementation deviated from the original design), **part project plan**
> (Part II: what comes next, in dependency order, with honest risk notes), and
> **part white paper** (the rationale for each piece, kept close to the math).
>
> The current *runtime* behavior is documented in [whitepaper.md](./whitepaper.md);
> the evaluation harness in [math-eval.md](./math-eval.md). This doc is the
> bridge between them: the record of the evolution program and the plan for the
> rest of it.
>
> The goal stated by the team: *"a quantitative overlay on project planning that
> goes beyond embeddings and RAG."* Embeddings give you semantic retrieval; tags
> give you interpretation; this layer turns those into per-person, per-iteration,
> per-theme quantitative signals usable for planning, coverage analysis, and
> prioritization.

---

# Part I — What we built (the manifest)

## 1. From rank-1 projection to anchored ridge

The model we evolved away from called itself a "factor model" but was,
mathematically, a **rank-1 per-issue projection** (whitepaper §2):

```
ê_i      = Tᵀ (Σ_shrunk · r_i)         // synthesized direction from tag loadings
û_i      = ê_i / ||ê_i||
factor_i = (e_i · û_i) · û_i           // 1-D projection, zeroed when e·û ≤ 0
residual = e_i − factor_i
```

Its structural limits, identified at the start of this program:

- No per-tag attribution of how text explains tags.
- No corpus-level "themes" — factors were per-issue lines, not shared axes.
- No "AI vs. embedding" disagreement signal.
- No mechanism for negative evidence ("this is *not* a Safari bug").

All four are now addressed by shipped code (§3 below), with one deliberate
twist: the rank-1 model was **retained, not deleted** — it has three remaining
jobs (§3.7).

## 2. Design constraints (still binding)

These shaped every decision and still hold:

1. **Positive loadings are the default.** Tags express "this issue is about X" —
   a non-negative claim. Negative loadings exist but require explicit,
   evidence-backed generation; they do not arise naturally from AI analysis.

2. **Factor model is an analogy, not a strict commitment.** We are not bound to
   textbook factor analysis with latent-factor identifiability or
   maximum-likelihood estimation. The math serves the product.

3. **Regression is fine.** Once negative loadings exist, ridge regression with a
   closed-form solve replaces NNLS (which we'd need only if positivity were
   enforced on `f`).

4. **Complexity is acceptable when it pays off in planning overlays.** The
   product goal is structured, quantitative signals for planning — not just
   better search ranking.

5. **The AI's `r_i` remains load-bearing.** It is the analyzer's interpretation
   of the issue and what users see in the UI. The math layer refines it; it does
   not replace it with a regression estimate.

6. **False negatives are worse than false positives.** Negative signal actively
   contradicts, so the bar for emitting it is strictly higher than for positive
   signal — evidence-gated, capped, and provenance-tracked (§3.2).

## 3. The shipped model

One diagram, matching what runs in production today:

```
                                       ┌──────────────────────────────────┐
  Issue text  ──►  AI analyzer  ─────► │  r_i⁺ ∈ [0,1]^T   positive tags  │
                       │               │  r_i⁻ ∈ [0,0.7]^T negated tags   │
                       │               │  evidence ranges per tag         │
                       │               └──────────────────────────────────┘
                       │                              │  verifier gate:
                       │                              │  quotes must resolve
                       │                              ▼
                       │            r_i = r_i⁺ − r_i⁻  ∈ [-0.7, 1]^T  (signed)
                       │                              │
                       ▼                              ▼
                embedding e_i ∈ ℝ^D       ┌───────────────────────────────┐
                       │                  │ Anchored ridge (diagonal Λ)   │
              corpus-mean centering       │ f_i = (TTᵀ + Λ)⁻¹(Te + Λr)    │
                       │                  │ λ_scored fixed, λ_unscored    │
                       └────────────────► │ GCV-selected per corpus,      │
                                          │ revision-cached               │
                                          └───────────────────────────────┘
                                                       │
                       ┌───────────────────────────────┼──────────────────────────────┐
                       ▼                               ▼                              ▼
                  Similarity                     Themes (debug tier):         Diagnostics
                  wF·cos(f_A,f_B)                NMF on F⁺ = max(0, f)        R² = 1−‖e−Tᵀf‖²/‖e‖²
                + wR·cos(res_A,res_B)            → W, H, stable IDs,          DriftCosine(f, r)
                  default in search /            labels, /debug/themes        → tag-health sweep
                  explore / people                                            → curation detector
```

### 3.1 Corpus-mean centering (foundation)

All decomposition math runs on corpus-mean-centered, re-unit-normalized
embeddings (whitepaper §2.0; `internal/issuemath/centering.go`). Two separate
means — one for the issue corpus, one for the tag catalog — because the two
populations have different common directions. The transform is runtime-only
(persisted vectors are never rewritten), means are revision-cached, and
external vectors (queries, person centroids) are centered with the **stored
corpus means, never their own**. Corpora below `MinCenteringVectors` (5) skip
centering entirely — a mean over a handful of vectors is noise that can make
near-parallel vectors antipodal.

Centering is a *precondition*, not part of the solve: `ComputeRidgeDecomposition`
and the GCV selector expect pre-centered inputs
(`ridge_decomposition.go`, `ridge_gcv.go`), and the λ cache centers its sample
with the same revision-cached means the ranker uses
(`internal/ridgelambda/cache.go`, `centeredEmbeddings`).

One consequence worth restating: centered tag-tag cosines are legitimately
signed. Negative entries encode genuine anti-correlation between tag
directions. This is why the originally planned "tighten Σ to `max(0, cos)`"
change was **dropped** — clamping would destroy that signal.

### 3.2 Signed relevance `r_i = r_i⁺ − r_i⁻` (as built)

The analyzer emits two vectors per issue. `r_i⁺` is today's tag relevance.
`r_i⁻` is explicit refutation, and it survives a pipeline designed to keep the
negative-signal bar high:

1. **Prompt.** The analyzer is instructed to return `negated_tags` only for
   direct textual negation ("this is not a regression"), with 1–3 verbatim
   quotes; entries without evidence are discarded, and the prompt warns the
   evidence will be cross-verified (`internal/ai/openai.go`).
2. **Normalization.** `normalizeNegated` (`internal/ai/service.go`) drops tags
   not in the taxonomy, drops entries with no evidence, clamps confidence to
   `[0, 0.7]` (the design's magnitude cap, enforced in code), and dedupes to
   highest confidence.
3. **Evidence gate.** `applyAnalyzerNegations`
   (`internal/issueenrichment/verify.go`) re-verifies every quote against the
   source text via `resolveEvidenceRanges` — a negation whose evidence cannot
   be located is **discarded**, not kept-with-flag. A minimum confidence of
   0.1 applies. A verified negation of a tag the analyzer did not positively
   assign is persisted as a synthetic `Relevance: 0` row, so the negative
   claim is auditable on its own.
4. **Verifier-sourced negation.** The verifier emits `r⁻` directly in two
   cases, both with provenance `verifier-dominance`:
   - **Anti-alignment**: centered cosine between issue and tag embedding below
     −0.05 → negation 0.5. This is the "generic-tag drag" suppressor — it only
     works because centering makes anti-alignment observable at all.
   - **Dominance**: an unassigned candidate tag whose alignment beats the
     assigned tag by ≥ 0.18 with alignment ≥ 0.35 (plus a specificity margin
     of 0.10 in candidate selection), while the assigned tag sits below 0.16
     → negation 0.25. The 0.25 magnitude is calibrated so
     `Relevance − 0.25` reproduces the historical ×0.75 down-rank shrink while
     keeping the positive AI signal intact.
   When analyzer and verifier both negate a tag, the magnitudes merge as
   `max(analyzer, verifier)` capped at 0.7, and analyzer provenance wins
   (it carries evidence).

**Persistence:** negation lives on `TagRelevance` as `Negation` /
`NegationProvenance` / `NegationEvidence` / `NegationReason`
(`internal/domain/tags.go`), serialized inside the existing `tag_scores_json`
JSONB blob — no dedicated columns, no migration. `EffectiveRelevance() =
Relevance − Negation` drives display ranking with a 0.2 cutoff
(`internal/issues/store.go`).

**Consumption in the math layer:** `signedAnchor`
(`internal/issuemath/ridge_decomposition.go`) builds
`anchor[k] = Relevance − Negation` and marks a tag "scored" if it was scored
*or* negated — so a negated tag gets the tight anchor penalty and is pulled
toward its signed value. This feeds all three ridge paths: the ranking
decomposition, GCV selection, and drift.

**Sources of `r⁻`, as they actually stand:**

| Source | Status | Notes |
|---|---|---|
| Analyzer negation detection | **Shipped** | Evidence-gated as above; provenance `analyzer-negation`. |
| Verifier anti-alignment / dominance | **Shipped** | Provenance `verifier-dominance`; magnitudes 0.5 / 0.25. |
| User dismiss actions | **Not built** | The `dismiss` provenance constant exists but has no producer. Note: the existing `/tags/dismiss` endpoint dismisses *tag-merge suggestions*, not per-issue tag relevance — an earlier version of this doc claimed otherwise. A per-issue "this tag is wrong" affordance does not exist yet. |
| Co-occurrence anti-correlation | **Partially adjacent** | `internal/tagcooccurrence` computes directed `ImplicitNegative` (shrunk lift) — but it feeds *search-time anti-correlation penalties* (wired in `internal/search/search_issues.go`) and issue tag-set coherence scoring (`internal/issuexray`), not `r⁻`. The `cooccurrence` provenance constant is unwired. It deliberately excludes synthetic `Relevance: 0` negation rows ("absence, not presence"). |
| Refinement contradiction | Deferred | Unchanged from original design. |
| Mutual-exclusion taxonomy | Optional | Unchanged from original design. |

### 3.3 Anchored ridge `f_i` (as built)

The shipped solve is the diagonal-penalty generalization of the original
design:

```
f_i = argmin_f  ‖e_i − Tᵀf‖² + Σ_k λ_k (f_k − r_ik)²
    = (T Tᵀ + Λ_i)⁻¹ (T e_i + Λ_i r_i),     Λ_i = diag(λ_k)

λ_k = λ_scored   (0.5, fixed)               if tag k was scored or negated on issue i
    = λ_unscored (GCV-selected per corpus)  otherwise
```

The two-penalty structure is the important as-built refinement over the
uniform-λ design: tags the analyzer expressed an opinion about (positive or
negative) are anchored firmly to that opinion; tags it stayed silent on get a
looser penalty and are free to be claimed by the geometry — which is exactly
what makes "missing tag" detection possible (§3.6).

**Computational shape** (`internal/issuemath/ridge_decomposition.go`):

- `ridgeSolver` builds `T` and the base Gram `T Tᵀ` **once per decomposition**
  — the O(K²·D) cost. (Rows of `T` are tag embeddings, so the K×K Gram is
  `TTᵀ`.)
- Per issue: copy the Gram, add `Λ_i` to the diagonal, form
  `rhs = T e_i + Λ_i r_i`, one Cholesky solve (`A = TTᵀ + Λ_i` is SPD). O(K²)
  per issue; microseconds at K ≈ 200. This is the **same factorization GCV λ
  selection uses**, so ranking and λ selection share one numerical path (Track
  D5); a defensive LU fallback with a process counter
  (`RidgeCholeskyFallbackCount`) covers the never-observed non-SPD case.
- The solver reuses scratch buffers and is therefore **not concurrency-safe**
  in itself; since WP-103 the corpus decomposition is computed once per
  revision behind a single-flight cache (`internal/ridgedecomp`), so the
  solver no longer runs per request and never runs concurrently.
- Per-issue outputs (`RidgeVectors`): unit loading `f`, unit reconstruction
  `Tᵀf`, unit residual `e − Tᵀf`, their pre-normalization norms, and an honest
  per-issue `R² = 1 − ‖e − Tᵀf‖²/‖e‖²` (allowed to go negative).
- Corpus outputs: `AggregateR2 = 1 − Σ‖resid‖²/Σ‖e‖²` (variance-pooled; the
  rank-1 model pools the same way despite a struct comment claiming "mean R²"
  — a doc-debt item, see Track D7), and blend weights
  `w_F = clamp(AggregateR2, 0.05, 0.95)`, `w_R = 1 − w_F`. Corpora below the
  5-issue decomposition floor use fixed fallback weights (0.4/0.6).

### 3.4 GCV λ selection and the revision cache

The single most important empirical lesson of the program: **the unscored
penalty is not a transferable constant.** The debug-endpoint default
`λ_unscored = 0.05` overfits — unscored tags soak up variance, R² inflates to
~0.90, and ranking *regresses*. The right value depends on tag-catalog
conditioning and the K/D ratio, so it is derived per corpus by generalized
cross-validation — no labels, no held-out split
(`internal/issuemath/ridge_gcv.go`):

```
λ*_unscored = argmin_{λ ∈ grid}  Σ_i  ‖e_i − Tᵀf_i(λ)‖² / (D − df_i(λ))²
df_i(λ)     = tr( (TTᵀ + Λ_i)⁻¹ TTᵀ )        (effective degrees of freedom)
grid        = {0.01, 0.03, 0.1, 0.3, 1.0, 3.0, 10.0}
```

Per grid point, each sampled issue costs a Cholesky factorization plus a trace
`df = tr(A⁻¹ TTᵀ)`. Rather than materialize the K×K product `A⁻¹·Gram`, this is
computed via the identity `df = K − Σ_k λ_k (A⁻¹)_kk` (since `TTᵀ = A − Λ` and
`Λ` is diagonal), reading only the diagonal of `A⁻¹` from the existing
factorization — ~2× cheaper than the old materialized solve at K≈200/D=1536 and
allocation-free per sample (Track D5). A grid point where `D − df ≤ 0` for any
sample is rejected outright: GCV is
unusable when effective df reaches the embedding dimension, i.e. the K ≥ D
regime silently falls back to rank-1.

**Caching** (`internal/ridgelambda/cache.go`): the selected λ is memoized per
corpus revision, invalidated on any corpus write via the shared revision
counter. The GCV sample is a deterministic stride sample over the ID-sorted
corpus, capped at 2000 issues; it is centered with the same revision-cached
corpus means the ranker uses. The cache returns `(0, false)` — the
fall-back-to-rank-1 signal — for corpora under `MinDecompositionIssues` (5),
tagless catalogs, or catalogs with no tag embeddings. On the eval fixtures GCV
picks `λ_unscored ≈ 3.0` (honest R² ~0.80).

### 3.5 Similarity in production: the default flip

`RidgeBlend` (`ridge_decomposition.go`) offers two similarity shapes; the
shadow harness settled the fork in favor of **tag space**:

```
sim(A,B) = w_F · cos(f_A, f_B)  +  w_R · cos(residual_A, residual_B)
```

with marginalization: a pair where one side has no factor evidence puts full
weight on the residual side, and vice versa; candidates missing from the
decomposition score as pure semantic similarity at full weight.

**Wiring:** one shared `ridgelambda.Cache` is threaded through the API into
search (`internal/map/search.go`, `WithRidgeSimilarity`), explore
(`internal/map/explore.go`, `WithExploreRidgeSimilarity`), and person
recommendations (`internal/people/person_detail.go`). All three default to
ridge whenever the cache yields a λ; otherwise they fall back to the rank-1
blend, and below that to the legacy fixed-weight cosine blend. There is no
feature flag — the "flag" is the cache's `(λ, ok)` return, which is exactly
the graceful-degradation behavior we want.

**Every downstream modifier is model-agnostic and unchanged**: the
tag-correlation factor nudge, generic-co-occurrence and region boosts,
anti-correlation penalties, the evidence clamp at zero, and the multiplicative
freshness / velocity / authority / specificity chain all operate on the
blended score regardless of which model produced it. This was a deliberate
design property — the flip changed one term, not the composition rule.

**What is *not* flipped:** person tag *profiles* (the displayed aggregation)
still average raw `r_i` weighted by specificity, not `f_i` — that switch is
Phase 5 work (§8, Track B). The map projection is untouched (it is PCA on tag
relevance, not on this decomposition — Track C).

### 3.6 Drift diagnostics and tag health

The design called for `drift(i) = ‖f_i − r_i‖`. The shipped metric is a
**restricted cosine** instead (`DriftCosine`, `internal/issuemath/ridgescore.go`):

```
DriftCosine(f, r) = Σ'_k f_k r_k / √(Σ'_k f_k² · Σ'_k r_k²)
```

where `Σ'` skips components at which *both* vectors are zero. Rationale: the
Euclidean norm is dominated by uniform least-squares shrinkage (`f` is
systematically smaller than `r`), which is not disagreement; the restricted
cosine is scale-invariant and measures *directional* disagreement only. Low
cosine = the AI's tagging points somewhere the geometry does not support.
This deviation is deliberate and should be treated as the canonical drift
definition going forward.

**The two-λ regime, on purpose:** the tag-health sweep
(`internal/diagnostics/debug_tag_health.go`) does **not** use the GCV λ. It
deliberately uses the fixed loose `λ_unscored = 0.05` so unscored tags float —
the GCV-selected penalty (~3.0) pins unscored tags toward zero, which is right
for *ranking* but would suppress exactly the "missing tag" candidates drift is
supposed to surface. Ranking and diagnostics legitimately want different
priors; this is documented here because it looks like a bug and is not.

Shipped surfaces:

- `GET /api/v1/debug/issues/{id}/ridge` — per-issue anchored ridge: per-tag
  `{anchor r_k, ridge f_k, delta}` sorted by |delta|, plus `DriftCosine`.
- `GET /api/v1/debug/tag-health` — corpus sweep via `ComputeCorpusDrift`:
  flags **open** issues with `DriftCosine < 0.5` that have at least one tag
  with |delta| ≥ 0.3; reports spurious tags (anchored, large negative delta)
  and missing tags (unanchored, large positive delta). Capped at 20 issues ×
  5 tags.
- **Curation:** the curation detector (`internal/curation/detect.go`) consumes
  the tag-health sweep as its **primary** mis-tagging signal, ahead of the
  rank-1 low-R² pass — which was demoted to what it is actually good at:
  discovering *uncovered concepts* (see §3.7).

### 3.7 The rank-1 model's remaining jobs

`factor_model.go` was retained rather than deleted (the original staging table
said "drop" — superseded). Its three jobs:

1. **Fallback ranker** for corpora too small or degenerate for GCV
   (`< 5` issues, no tag embeddings, K ≥ D).
2. **Debug backing** for `GET /debug/factor-weights` and
   `GET /debug/issues/{id}/r2`.
3. **Residual-cluster concept mining** (`internal/issuemath/residual_clusters.go`,
   `internal/memories/synthesizer.go`): cluster issues whose rank-1 R² < 0.15
   by residual-direction cosine (single-link, neighbor threshold 0.3, size
   floor 3), excluding already-conceptualized issues, and LLM-propose a new
   project concept per cluster — propose-only, routed through the human-review
   loop. This is a genuine consumer of the *residual* geometry that the
   evolution program created room for: the rank-1 residual answers "what
   concept do the tags fail to capture?", a different question from ridge
   drift's "which tags disagree with the geometry?".

Note the rank-1 anchor is **positive-only** — `synthesizeFactorEmbedding` reads
`Relevance` and ignores `Negation`. The fallback model does not see signed
relevance. Acceptable for a fallback; recorded here so nobody is surprised.

### 3.8 Themes: the corpus factorization service (shipped, debug tier)

Track A / plan Stage 2, delivered end-to-end (PRs #206, #207, #212). Two
layers: `internal/issuethemes` stays a pure, deterministic NMF library;
`internal/themes` is the service that makes it a corpus artifact.

**The factorizer** (`internal/issuethemes/themes.go`):

- Input `V = F⁺`: per-issue ridge loadings rectified to `max(0, f_i)`; rows
  with no positive mass dropped.
- **Initialization:** NNDSVD (Boutsidis–Gallopoulos) via thin SVD — component 0
  from `|u₀|√σ₀`, later components from the dominant sign-split of each
  singular vector pair; a deterministic mean-based seed fills degenerate
  components. No randomness anywhere; determinism is tested bit-for-bit.
- **Updates:** Lee–Seung multiplicative updates (Frobenius objective) with an
  early stop — relative Frobenius improvement < 1e-4, iteration cap 200 as a
  backstop. The original fixed-50 loop was *under-converged* (fixtures
  converge at 58–114 iterations), so the early stop improved factorization
  quality, not just cost. `Result` carries `Iterations` and
  `ReconstructionError`.
- **Post-processing:** each theme's tag distribution `H_k` is L1-normalized
  (mass pushed into `W`), themes ordered by total `W` mass, top-5 tags per
  theme, and a unit-normalized centroid `v_k = unit(Σ_t H_kt · tag_embedding_t)`.
- **K = 8** (clamped to `min(K, N, T)`), kept by a dated sweep decision
  (2026-07-02, plan WP-206): reconstruction error falls monotonically with K,
  so the elbow is a weak criterion — identity *stability* is the
  discriminating signal, and it cliffs immediately above the corpus's true
  structure count (churn 8–26 events/12 refreshes at K=9–12 vs zero at
  K=7–8). Adaptive K remains unauthorized.

**The service** (`internal/themes`):

- **Loadings + participation:** raw `f` recovered from the decomposition
  cache (`Loading · LoadingNorm`); an issue participates when it has a usable
  embedding AND at least one anchored tag; floor
  `minThemeParticipants = 16` (2·K), below which the cache degrades to
  `(zero, false)` like every other math cache.
- **Revision-keyed cache** mirroring `ridgelambda`: single-flight
  read-through compute per corpus revision.
- **Identity stability:** Hungarian matching of new-vs-previous theme tag
  rows by cosine (threshold 0.6, keyed by tag name over the union so catalog
  changes align); matched themes inherit stable string IDs, unmatched mint
  monotonically, retired IDs are reported. Identity state survives degenerate
  revisions. Soak-tested: zero churn across 12 mutation refreshes. Caveat:
  matching runs on the top-5 tags the library exports, not full `H` rows —
  remediation is plan WP-207.
- **Labeling:** `ThemeLabeler` in `internal/ai` — a deterministic top-tag
  fallback label attaches synchronously on mint; the LLM name arrives via a
  background job (generation-stale writebacks discarded). Relabel only when
  the matched row drifts below top-tag cosine 0.5 (< the 0.6 identity
  threshold), keeping `previousLabel`. Labels and identity are in-memory:
  restarts relabel — persistence is plan WP-406 and gates any promotion out
  of debug tier.
- **Debug API:** `GET /debug/themes` (stable IDs, weight shares, top tags,
  match diagnostics, refresh telemetry), `GET /debug/themes/{id}`
  (centroid-nearest and top-W issues), `GET /debug/issues/{id}/themes`
  (per-issue W row). Qualitative read on the matheval fixture corpus: 8/8
  themes recovered a real domain group. The dev-corpus read and soak are plan
  WP-208.

## 4. Evidence: what the numbers say (and don't)

The evaluation harness (`internal/matheval`, [math-eval.md](./math-eval.md))
runs two fixtures over the *same* 16 tags / 48 issues / 32 fully-judged
queries and the same judgment set. The **synthetic** fixture synthesizes
embeddings as relevance-weighted tag sums (dim 24); the **real** fixture
(WP-301) re-embeds the identical tag descriptors, issue texts, and query texts
with the production `text-embedding-3-small` model (dim 1536), committed to
`testdata/real_embeddings.json` so CI needs no network. Both fixtures, both
similarity paths, are asserted on every `go test` run (four baseline entries).

**Synthetic fixture** (embeddings built from tags — a floor argument):

| Measurement | NDCG@8 | Recall@8 | Where |
|---|---|---|---|
| Pre-centering baseline | 0.823 | 0.855 | whitepaper §2.0 |
| Rank-1, centered (golden baseline) | 0.8658 | 0.9117 | `baseline.json` → `synthetic.rank1` |
| Ridge, GCV λ_unscored = 3.0, tag-space (golden baseline) | **0.9309** | **0.9586** | `baseline.json` → `synthetic.ridge` |
| Full-path A/B (ridge − rank-1) | **+0.0651** | **+0.0469** | both rows above, asserted every run |
| Explore rank-1 / ridge, full-path (WP-302) | 0.9275 / 0.9260 | 0.8681 / 0.8738 | `synthetic.explore` — ridge ≈ ties (NDCG −0.0015, Recall +0.0057) |
| Person rank-1 / ridge, full-path (WP-302) | 0.5367 / 0.6279 | 0.4381 / 0.4488 | `synthetic.person` — ridge **wins** (NDCG +0.0912) |

**Real fixture** (production `text-embedding-3-small` geometry, WP-301):

| Measurement | NDCG@8 | Recall@8 | Where |
|---|---|---|---|
| Rank-1, centered (golden baseline) | **0.9050** | **0.8688** | `baseline.json` → `real.rank1` |
| Ridge, GCV λ_unscored = 0.01, tag-space (golden baseline) | 0.7536 | 0.5929 | `baseline.json` → `real.ridge` |
| Full-path A/B (ridge − rank-1) | **−0.1514** | **−0.2759** | both rows above, asserted every run |
| Ridge tag-space, similarity-only (shadow) | 0.7910 | 0.6472 | `ridge_shadow_test.go` — *regression* |
| Ridge recon-space, similarity-only (shadow) | 0.8744 | 0.8294 | shadow harness — still below rank-1 |
| Ridge tag-space, GCV, **uncentered** (shadow) | 0.9514 | 0.9112 | shadow — centering *hurts* here |
| Explore rank-1 / ridge, full-path (WP-302) | 0.8057 / 0.5784 | 0.7228 / 0.5426 | `real.explore` — ridge **regresses** (NDCG −0.2273) |
| Person rank-1 / ridge, full-path (WP-302) | 0.5594 / 0.4984 | 0.4726 / 0.4589 | `real.person` — ridge **regresses** (NDCG −0.0610) |

Reproduce with `go test ./internal/matheval -run TestRidgeShadowComparison
-ridge -v` (both fixtures) and `go test ./internal/matheval -run TestMathEval
-v` (committed baselines). Regenerate the real vectors with `go run
./internal/matheval/cmd/embedfixture` (needs `OPENAI_API_KEY`).

Honest caveats, all of which are Track D work:

1. **Both similarity paths are now under the golden baseline.** WP-101 grew
   `matheval/testdata/baseline.json` from one entry to two — `rank1` (search
   with no options, the fallback path) and `ridge` (the shipped default: the
   eval injects `WithRidgeSimilarity` at the GCV-selected λ, mirroring the API
   layer's `ridgelambda.Cache` injection). Both are asserted on every ordinary
   `go test` run with no opt-in flag, so a change that regresses the production
   ridge blend fails CI. The GCV λ is re-derived each run and recorded for
   observability only (`ridge.gcvLambdaUnscored`); it is never hardcoded in the
   assertions, so a grid or GCV change surfaces as a metric delta, not silent
   drift.
2. **The full-path A/B deltas are now pinned, not a run log.** The ridge shadow
   comparison (`ridge_shadow_test.go`, opt-in `-ridge`) survives for its
   similarity-shape and fixed-λ overfit sweeps, but the numbers that gate
   regressions are the two committed baseline rows above, not its log output —
   both derive λ through the same `ridgeGCVFixture` helper.
3. **On real embedding geometry, the ridge tag-space win does not transfer — it
   inverts. This is the most important negative result in the program so far,
   and it is stated here without softening.** WP-301 re-embedded the identical
   fixture texts with the production `text-embedding-3-small` model and re-ran
   the full comparison. Every ridge-vs-rank-1 number before this used embeddings
   *constructed from tags*, where a tag-space model wins by construction — a
   floor argument. On real geometry:

   - **The shipped ridge default (centered, tag-space, GCV λ) regresses ranking
     substantially: NDCG@8 0.7536 vs rank-1's 0.9050 (−0.1514), Recall@8 0.5929
     vs 0.8688 (−0.2759)**, full-path. It regresses similarity-only too (0.7910
     vs 0.8867). The synthetic fixture's +0.065 NDCG win became a −0.15 loss.
   - **Tags explain almost none of the real embedding variance.** Aggregate R²
     collapses from ~0.80 (synthetic ridge) to **~0.16**; the 16 tags span a
     16-dim subspace of a 1536-dim space, so projecting an issue onto tag-space
     discards ~84% of its semantic signal. Because the blend weights the factor
     term by that R², the tag term is *already* near-silent on a real corpus —
     but the ridge tag-space similarity shape still underperforms rank-1's
     factor+residual blend on what remains.
   - **GCV selected λ_unscored = 0.01, the grid floor (censored — it wants even
     lower)**, versus 3.0 on the synthetic fixture. GCV correctly reads the real
     geometry as one where the penalty should be minimal; it is *not* a
     fixture-fit constant, exactly as designed. It simply cannot rescue a
     similarity shape that discards the residual.
   - **Two shape/preprocessing findings that flipped sign versus synthetic:**
     reconstruction-space similarity (0.8744) beats tag-space (0.7910) on real
     geometry — the opposite of the Phase 3b synthetic verdict that picked
     tag-space; and **centering *before* the tag-space ridge hurts** on real
     embeddings (uncentered tag-space GCV scores 0.9514 vs centered 0.7910).
     Neither should be read as a new default — they are single-fixture signals —
     but both say the shipped configuration was tuned on a geometry that does not
     resemble production.

   **Interpretation against the pre-committed rule (this plan's §WP-301 risk):**
   ridge does not beat rank-1 on real geometry within noise — it loses cleanly
   and by a wide margin on the shipped tag-space path. Per the pre-commitment,
   the ridge-default flip now warrants re-examination: its cost is complexity,
   and on real geometry the complexity is not buying a ranking win. Stages 4–5
   still stand — they consume `f_i` for *structure* (themes, drift, map input),
   not for ranking wins, and the drift signal (analyzer-vs-geometry divergence)
   is a different quantity than ranking NDCG. But any claim that "ridge improves
   search ranking" must now cite the real-fixture rows, where it does not.

   **Layer caveat (spec's own escalation rule).** This is *layer 1*: real-model
   embeddings over texts that were themselves generated *from* the tag structure.
   Those texts are more realistic than tag-sum vectors but are still one step
   removed from a real corpus. Layer 1 discriminated sharply (it did not
   saturate), so the layer-2 curation of a real anonymized corpus is *not*
   forced by a null result — but the direction it points (real geometry demotes
   the tag-space ranking story) is the signal layer 2 would sharpen, not
   reverse.
4. **Explore and person recommendations are now harness-validated too (WP-302),
   and they corroborate the real-geometry verdict above.** Both surfaces share
   the blend + ridge default but not the query shape — explore is seeded by an
   issue, person fit by a profile — so each got its own judged fixture (12 explore
   seeds; 6 synthetic person histories over the existing 48-issue corpus) with
   mechanical grades derived from the generator's tag-domain ground truth, both on
   both models and both fixtures, asserted every `go test` run. The rows are in
   the two tables above. The pattern matches search exactly: on the **synthetic**
   fixture ridge ties explore and wins person (+0.09 NDCG); on the **real** fixture
   ridge **regresses both** — explore −0.2273 NDCG / −0.1802 Recall, person −0.0610
   / −0.0137. So the ridge default's real-geometry loss is not a search-only
   artifact — it reproduces across all three ranking surfaces. Explore seeds by an
   issue embedding (not a query text), so the geometry is not identical to search,
   yet it lands the same direction, which strengthens rather than complicates the
   WP-304 read. Caveat: the explore/person judgments are mechanically derived from
   the same tag-domain ground truth the synthetic embeddings are built from, so on
   the synthetic fixture they are circular in the same way; the real-fixture rows
   are the load-bearing evidence. The reproduce commands are `go test
   ./internal/matheval -run TestMathEval -v` (asserts every row) — see the
   `explore`/`person` baseline keys.

## 5. Deviations ledger: design vs. as-built

Where the implementation deliberately diverged from this document's earlier
design, and why. These are decisions, not drift — the as-built column wins.

| Designed | As built | Why |
|---|---|---|
| Tighten Σ to `max(0, cos)` | **Dropped** | Corpus-mean centering landed first; centered cosines are legitimately signed — clamping would destroy real anti-correlation signal. |
| `drift(i) = ‖f − r‖` | `DriftCosine` (restricted cosine) | Euclidean drift is dominated by uniform ridge shrinkage, which is not disagreement. Scale-invariant directional drift is the meaningful signal. |
| Uniform λ | Diagonal `Λ`: fixed `λ_scored = 0.5`, GCV `λ_unscored` | Scored and unscored tags need different priors; the unscored penalty is corpus-dependent, not a constant. |
| One λ everywhere | Two λ regimes: GCV for ranking, fixed 0.05 for drift | GCV pins unscored tags toward zero — right for ranking, fatal for missing-tag detection. |
| "Drop `factor_model.go`" | Retained | Fallback for small/degenerate corpora, debug R² endpoints, and residual-cluster concept mining. |
| `DownRank` multiplies relevance ×0.75 | Emits `r⁻ = 0.25` | Same effective shrink, but the negative evidence is explicit, signed, provenance-tracked, and visible to the ridge anchor. |
| Dismiss provenance sourced from `/tags/dismiss` | **Unwired** | That endpoint dismisses tag-merge suggestions, not per-issue relevance. A real dismiss affordance is future work (Track D). |
| NMF "~50 iterations to convergence" | Early stop: relative Frobenius improvement < 1e-4, cap 200 (WP-206) | The fixed-50 loop shipped first was under-converged (fixtures converge at 58–114 iterations); the early stop restored the design intent and improved quality. |

Documentation debt created by the flip — **resolved by WP-102**: the ridge
constants header now teaches the two-λ regime, whitepaper §10.3 describes the
shipped default, and the rank-1 aggregate comment says pooled, not mean.

## 6. Status board

| Phase | Status | Where |
|---|---|---|
| **Phase 0** — foundations | **Done (reshaped).** Σ tightening dropped in favor of corpus-mean centering; drift shipped as `DriftCosine`. | `internal/issuemath/centering.go`, `ridgescore.go` |
| **Phase 1** — signed relevance | **Shipped end-to-end.** Analyzer negation with evidence gate, verifier negations (anti-alignment 0.5 / dominance 0.25), 0.7 cap honored on every path, JSONB persistence. Emitting: `analyzer-negation`, `verifier-dominance`. Dead constants: `dismiss`, `cooccurrence`. | `internal/ai/*`, `internal/issueenrichment/verify.go`, `internal/domain/tags.go` |
| **Phase 2** — ridge shadow | **Shipped.** Per-request anchored ridge behind `GET /debug/issues/{id}/ridge`, signed anchor, diagonal penalties. | `internal/issuemath/ridgescore.go`, `internal/diagnostics/debug_ridge_score.go` |
| **Phase 3** — default flip | **Shipped.** GCV λ revision-cached (`internal/ridgelambda`, stride-sampled ≤2000, centered with corpus means); search, explore, and person recommendations default to ridge tag-space blend with rank-1 fallback; downstream modifiers untouched. All three surfaces now under the golden baseline: search since WP-101, explore + person recommendations since WP-302 (both models, both fixtures) — and on real geometry all three regress under ridge, the evidence WP-304 acts on. | `internal/issuemath/ridge_decomposition.go`, `ridge_gcv.go`, `internal/ridgelambda/`, `internal/map/search.go`, `explore.go`, `internal/people/person_detail.go` |
| **Phase 3.5** — drift consumers | **Shipped (beyond original plan).** Tag-health sweep at fixed loose λ; curation detector uses drift as primary mis-tagging signal; rank-1 residual clusters demoted to uncovered-concept mining (propose-only). | `internal/diagnostics/debug_tag_health.go`, `internal/curation/detect.go`, `internal/issuemath/residual_clusters.go`, `internal/memories/synthesizer.go` |
| **Phase 4** — themes | **Shipped to debug tier (plan Stage 2, PRs #206/#207/#212).** Revision-keyed theme cache over corpus ridge loadings; stable identities (Hungarian match, threshold 0.6); LLM labels with deterministic fallback; NMF early-stop + per-refresh telemetry; K=8 kept by a dated sweep decision; three `/debug/themes` endpoints. Open follow-ons: full-H export (WP-207), dev-corpus soak (WP-208), identity/label persistence (WP-406). | `internal/issuethemes/`, `internal/themes/`, `internal/diagnostics/debug_themes.go` |
| **Phase 5** — planning overlays | Not started. Person profiles still aggregate raw `r_i` (specificity-weighted). | — |
| **Phase 6** — map on themes | Not started. Map remains PCA on `X·Σ` with sign convention + Procrustes alignment (whitepaper §3.3). | `internal/issuemath/projection.go` |

---

# Part II — What comes next (the plan)

> Execution detail for everything below lives in [math-plan/](./math-plan/README.md)
> — a sequenced work-package queue with per-package context, design, steps,
> validation, and acceptance criteria. This part is the strategy view; that
> directory is the working plan. When they disagree, the plan directory is
> more current.

Four tracks. A → B is the product arc (themes, then overlays); C is the riskiest
UX change and goes last; D is the hardening track that runs alongside and — in
two places — gates A. Estimates assume the established pattern-reuse (the
revision-cache shape already exists twice).

## 7. Track A — Themes to production (finish Phase 4)

> **Delivered (2026-07-02).** All six items shipped and merged as plan Stage 2
> (WP-201–206; PRs #206, #207, #212); the as-built description now lives in
> §3.8 and the per-WP outcomes in
> [math-plan/20-stage-2-themes.md](./math-plan/20-stage-2-themes.md).
> Follow-ons discovered while shipping: full-H export (WP-207), dev-corpus
> soak with an owner (WP-208), identity/label persistence gating Stage-4
> promotion (WP-406). The list below is retained as the original scope
> record.

The factorizer exists; what's missing is everything around it. In dependency
order:

**A1. Corpus decomposition source (gated by D4).** `issuethemes.Build` needs
per-issue ridge loadings for the whole corpus. Today the ridge decomposition is
recomputed per request and never persisted — computing it again per theme
refresh is affordable (one Gram build + N × O(K²) solves), but the right move
is a shared revision-keyed decomposition cache (D4) that themes, tag-health,
and ranking all read. Build A1 and D4 as one piece.

**A2. Revision-keyed theme cache.** Mirror `ridgelambda.Cache` exactly: a
`themes.Cache` holding the last `Result` keyed by corpus revision, computed
read-through on first access after a bump. No DB table yet — NMF at Sortit
scale is milliseconds once loadings exist, and in-memory-per-revision matches
the two existing caches. *Defer* durable persistence until a product feature
needs historical snapshots (theme drift over time, §8.3, is the first one that
does — see B3).

**A3. Theme identity stability across refreshes.** This is the map-Procrustes
lesson applied to themes, and it must land **before any UI**: NMF re-run on a
new revision can permute themes (and NNDSVD only mitigates, not prevents,
component reshuffling as the corpus changes). Match new `H` rows to previous
`H` rows by cosine (greedy or Hungarian at K = 8; trivial either way), carry
stable theme IDs forward, and only mint a new ID when no previous theme matches
above a threshold. Without this, "theme 3" silently becomes a different theme
between page loads and every overlay built on it is garbage.

**A4. API endpoints.** Theme list (`H` top tags, weight, centroid-nearest
issues), theme detail, and per-issue theme weights (`W` row). Read-only,
debug-tier first (`/debug/themes`), promoted once A3 proves stable.

**A5. Labeling.** Top-5 tags are the fallback label. The better label is a
one-shot LLM name from top tags + centroid-nearest issue titles — the exact
pattern residual-cluster concept mining already uses (`ProposeConceptFromCluster`),
so reuse that surface, primed with the project concept frame.

**A6. K and quality telemetry.** Log reconstruction error and H-row stability
(A3 match scores) per refresh. Keep K = 8 fixed until that telemetry says
otherwise. Add a convergence check to the MU loop (relative Frobenius
improvement < ε → stop early) while in there — cheap, removes the fixed-50
simplification.

Estimated: A1+D4 2–3 days; A2–A4 2–3 days; A5–A6 1–2 days. All backend;
no user-facing risk until A4 promotes out of debug.

## 8. Track B — Planning overlays (Phase 5)

Each overlay is computable from `f_i`, `W`, and existing lifecycle primitives.
Depends on Track A (through A4).

**B1. Per-person profile on validated loadings.**
`profile(person) = mean of f_i` over the person's issues (tag-space view) and
`mean of W_i` (theme-space view). Ship as a *parallel* profile next to the
existing raw-`r_i` specificity-weighted profile, compare, then decide whether
to replace or keep both — the existing profile is user-visible and the switch
deserves its own shadow window, exactly like the ridge flip got.

**B2. Per-iteration coverage.** `coverage(iteration) = normalized Σ W_i` over
the iteration's issues → "this iteration is 40% theme-3, 25% theme-5, …".
Coverage gaps become directly visible. Pure read on A4 outputs.

**B3. Theme drift across time.**
`drift(t) = 1 − cos(mean W in [t−Δ, t], mean W in [t−2Δ, t−Δ])`. This is the
overlay that forces durable theme persistence (A2's deferral ends here): either
persist `W` snapshots per refresh, or recompute historical windows from issue
snapshots. Persisting per-refresh aggregates (mean `W` per window, not per-issue
rows) is the cheap sufficient version — decide when B3 is scheduled, not before.

**B4. Gap analysis.**
`gap_score(k) = specificity(theme k) × (1 − recent coverage(k)) × historical authority(k)`
— themes that used to matter and have gone quiet. Combines A4 with existing
specificity and authority primitives.

**B5. Person-to-theme fit.** `fit(person, k) = cos(profile(person), H_k)` for
routing recommendations — surfaced as a suggestion, never automatic assignment.

UI dominates this track (1–2 weeks including it). Backend for B1–B2 and B4–B5
is days once A lands. B3 adds the persistence decision.

## 9. Track C — Map projection on themes (Phase 6)

Switch map positions from PCA on `X·Σ` (issue-by-tag relevance) to PCA on `W`
(issue-by-theme weights) — or, since `W` is already K ≈ 8-dimensional, possibly
a direct 2-D reduction of `W`.

Why last: it is the highest-risk user-facing change (the map is muscle memory),
and its input only stabilizes once A3 (theme identity) has soaked. Requirements
carried over from the current projection, all non-negotiable:

- Keep the Procrustes alignment chain against the previous layout — theme-space
  positions must not re-scramble the map on every refresh.
- Keep the old projection behind a debug flag for side-by-side comparison.
- Edges are untouched: they are uncentered-cosine embedding thresholds
  (whitepaper §3.2), independent of positions, and out of scope.
- Add a map-quality check to matheval *before* flipping (neighborhood
  preservation between old and new layouts at minimum) — the harness currently
  has zero map coverage, so today we could not even measure the regression this
  might cause.

Estimate: 1–2 days of math, but gated on the dev-corpus identity soak
(plan WP-208) and the matheval map metric (WP-303). Do not schedule it
earlier than that.

## 10. Track D — Hardening and honesty

The audit-driven track. Ordered by how much risk each item retires.

**D1. Put the production default under the golden baseline. (Done, WP-101.)**
Both paths guarded on every test run: `baseline.json` carries keyed `rank1`
and `ridge` entries (ridge injected at the GCV-selected λ exactly as the API
layer does; the λ recorded for observability, never asserted). Measured: rank1
0.8658/0.9117, ridge 0.9309/0.9586 — the full-path delta (+0.0651/+0.0469)
is now pinned in CI. A tamper test proved the guard bites (λ_scored=3.0 fails
ridge, leaves rank1 green).

**D2. A real-embedding fixture.** The synthetic corpus generates embeddings as
tag sums, structurally favoring ridge. Capture a small anonymized real corpus
(or real-model embeddings over fixture texts) with judged queries. Until then,
treat every fixture delta as a floor argument, not a measurement.

**D3. Eval coverage for explore and person recommendation.** Both inherit the
ridge blend with zero harness coverage. Explore needs a seeded-neighbor
judgment set; person-fit needs a small assignment-history fixture.

**D4. Revision-keyed decomposition cache. (Done, WP-103, PR #204.)**
`internal/ridgedecomp.Cache` (ridgelambda shape, single-flight, full corpus;
~135 MiB/revision at the 10k×K200×D1536 bound after dropping the
reconstruction vectors). All four API surfaces resolve ridge as: decomposition
cache → λ-only in-place → rank-1. The cached bundle carries the tag space
(`DecomposeQuery`) so queries, targets, and person centroids decompose into
the cached basis. Equivalence to the per-request path proven to 1e-9;
8-goroutine race test asserts one compute per revision. Deferred from this
item: the covariance double-build (lives in the rank-1 path, needs a
signature change there).

**D5. GCV cost and consistency. (Done, WP-105.)** Two smaller solver items,
both landed: (a) the trace `tr(A⁻¹·Gram)` no longer materializes the full K×K
product — it uses `df = K − Σ_k λ_k (A⁻¹)_kk`, reading only `diag(A⁻¹)` from the
factorization (~2× faster; a full λ-recompute at the K=200/D=1536/2000-sample
bound drops from ≈70s to ≈46s extrapolated, with the trace step itself 3.5ms→1.8ms
per sample). Note: the spec's suggested `‖L⁻¹T‖²_F` form is *slower* here because
D≫K makes it O(K²D); the diagonal identity was the right choice. (b) the ranker
now solves via Cholesky, the same factorization GCV uses, so λ is selected on the
path that ranks — with a defensive LU fallback (`RidgeCholeskyFallbackCount`,
never fires while Λ ≥ 0.05 keeps A SPD). matheval baselines unmoved (rank1
0.8658/0.9117, ridge 0.9309/0.9586).

**D6. Negation completion or cleanup.** Decide the two dead provenances:
build a real per-issue tag-dismiss affordance emitting `dismiss` negations
(high precision, low volume — still the best future source), and either wire
co-occurrence `ImplicitNegative` into background `r⁻` under the §3.2
operational rules or delete the `cooccurrence` constant. (The tag-relevance
copy paths were audited for negation-field loss and are correct — whole-struct
assignment carries the value-type provenance fields; only the pointer/slice
fields need the explicit re-copies they already have.)

**D7. Stale-text sweep.** Done (WP-102): constants header, whitepaper §10.3,
the "mean R²" comment, and two further stale ridge-mode comments found by
sweep (`RidgeSimilarityMode` docs, shadow-test header).

**D8. Standardize `TagDrift.Delta`. (Done, WP-104, PR #203.)**
`TagDrift.ZDelta` (nil under guards: < 20 observations or std < 1e-6),
standardized in a second pass over the corpus sweep. Tag-health gates on the
raw floor AND |z| ≥ 2.0 when a z exists; z-scored candidates rank ahead of
raw-only ones. The curation detector consumed the new ordering with zero code
change.

**D9. Inherited open weak spots** (whitepaper §10.2, still open, unowned by
any phase): Ledoit–Wolf (or explicitly-documented heuristic) covariance
shrinkage; the asymmetric authority/hubness slopes (0.25 vs 0.15); adaptive
`k` in tag specificity. None block A–C; schedule opportunistically.

## 11. Sequencing

```
D1, D4, D5, D7, D8, A1..A6: shipped and merged (Stages 1–2 of the plan).

D2 (real fixture) ─► D3 (explore/person eval) ─► B1..B5 overlays (debug tier)
WP-207 (full H export) ──────────────────────────┘  (before B2/B4 build on top-5 H)
WP-406 (identity+label persistence) ─┬─► promotion of any overlay out of debug
WP-208 (dev-corpus soak) ────────────┴─► C (map on W)  ◄── D3's map metric (WP-303)
D6, D9: parallel, opportunistic; D2 before believing any new fixture delta.
```

Rules of the road, unchanged: every item is a small stacked PR; math changes
ship dark or debug-tier first; anything user-visible gets a shadow window and
an eval metric before the flip — that discipline is what made the ridge flip
safe, and it is the template.

## 12. Open mathematical questions

Genuinely unresolved, kept visible so they get decided by evidence rather than
by default:

1. **Theme count K. (Decided 2026-07-02, WP-206 — keep K = 8.)** The sweep
   answered the question: reconstruction error falls monotonically with K, so
   the elbow alone is a weak criterion; identity stability is the
   discriminating signal, and it cliffs immediately above the corpus's true
   structure count. Adaptive K stays unauthorized (K above structure ⇒ ghost
   themes reshuffling every refresh). Reopen only if real-corpus telemetry
   shows reconstruction error stuck high AND zero churn at K=8.
2. **NMF convergence. (Resolved in the small, open in the large.)** Early
   stop shipped (WP-206): relative Frobenius improvement < 1e-4, cap 200 as
   backstop — the old fixed-50 was under-converged. Still open: whether MU is
   the right algorithm at larger N (HALS converges faster). Trigger recorded
   in the plan: iteration-cap hits or refresh latency that matters; max
   observed is 114 iterations, nowhere near firing.
3. **Rectification loss.** Themes see only `F⁺ = max(0, f)`. Negative loadings
   (refutations, anti-correlations) are invisible to theme structure. Probably
   correct — themes answer "what is this about" — but worth revisiting if
   refutation volume grows.
4. **GCV in the K ≥ D regime.** `D − df ≤ 0` rejects grid points; a large
   catalog on small embeddings silently falls back to rank-1. Fine today
   (K ≈ 200 ≪ D = 1536); needs a real answer (restricted GCV, or df computed
   on a tag subset) before any small-dimension embedding model is adopted.
5. **Blend-weight discontinuity at the fallback boundary.** Both models pool
   variance for their aggregate R² (they are the same statistic — a rank-1
   struct comment claiming "mean R²" is misleading text, Track D7), but a
   corpus crossing the 5-issue decomposition threshold jumps from the fixed
   fallback weights (0.4/0.6) to the clamped data-driven weight. Quantify the
   jump on a growing corpus or smooth the transition.
6. **Negative-evidence calibration.** The verifier magnitudes (0.5
   anti-alignment, 0.25 dominance) and the 0.7 cap are principled but
   hand-set. Once drift and dismiss data accumulate, these should be fit
   against observed correction rates rather than kept as constants.

## 13. What this is *not* solving

Unchanged in spirit from the original design, restated against current reality:

- **Outcome supervision.** Which themes predict resolution time, duplicate
  rates, or effort requires supervised regression on `W` against labeled
  outcomes. Follow-on, not part of this program.
- **Embedding model quality.** Garbage in, garbage out — if tag embeddings are
  noisy, `f_i` is noisy. Out of scope; worth its own review.
- **The hash-based embedding fallback.** Still exists (`runtime.go`), now
  instrumented and observable (whitepaper §9), still a soft correctness risk.
  This program neither uses nor fixes it.
- **Verifier threshold calibration.** The keep/flag/downrank thresholds
  (0.08/0.16/0.18/0.35) predate this program and remain hand-set; §12 item 6
  covers only the negation magnitudes.
- **Curation and memory math.** The librarian's scoring (staleness, redundancy,
  quiet-memory detection) has no documented math in either this doc or the
  whitepaper — a real documentation gap, but a different document.

---

## Appendix A — Notation (as built)

| Symbol | Definition | Domain |
|---|---|---|
| `e_i` | Issue embedding, corpus-mean centered + re-unit-normalized before all decomposition math | ℝ^D |
| `T` | Tag-embedding matrix, rows are (centered) tag embeddings | ℝ^(K×D) |
| `r_i⁺` | Analyzer-positive tag relevance | [0,1]^T |
| `r_i⁻` | Verified negation (analyzer or verifier sourced), capped | [0, 0.7]^T |
| `r_i` | Signed anchor `r_i⁺ − r_i⁻` | [−0.7, 1]^T |
| `Λ_i` | Per-issue diagonal penalty: `λ_scored = 0.5` on scored/negated tags, GCV `λ_unscored` elsewhere | diag ≥ 0 |
| `f_i` | Anchored ridge solution `(TTᵀ + Λ_i)⁻¹(Te_i + Λ_i r_i)` | ℝ^T |
| `R²_i` | `1 − ‖e_i − Tᵀf_i‖² / ‖e_i‖²` (honest; may be < 0) | (−∞, 1] |
| `DriftCosine(f, r)` | Cosine over components where not both are zero — canonical drift; replaces the designed `‖f − r‖` | [−1, 1] |
| `λ*_unscored` | `argmin_λ Σ_i ‖e_i − Tᵀf_i(λ)‖²/(D − df_i(λ))²`, grid-searched, revision-cached | grid |
| `df_i(λ)` | `tr((TTᵀ + Λ_i)⁻¹ TTᵀ)` — effective degrees of freedom | [0, K] |
| `F⁺` | Issue-by-tag matrix of `max(0, f_i)` | ℝ₊^(N×T) |
| `W`, `H` | NMF factors `F⁺ ≈ WH`; `H` rows L1-normalized | ℝ₊^(N×K), ℝ₊^(K×T) |
| `v_k` | Theme centroid `unit(Σ_t H_kt · tag_embedding_t)` | ℝ^D |
| `ê_i`, `factor_i`, `residual_i` (rank-1) | Retained in the fallback model and residual-cluster mining only | — |

## Appendix B — Code map

| Concept | Where |
|---|---|
| Corpus-mean centering | `internal/issuemath/centering.go`, `internal/centering` (revision cache) |
| Anchored ridge solve (single issue) | `internal/issuemath/ridgescore.go` |
| Corpus ridge decomposition + blend | `internal/issuemath/ridge_decomposition.go` |
| GCV λ selection | `internal/issuemath/ridge_gcv.go` |
| λ revision cache | `internal/ridgelambda/cache.go` |
| Drift sweep | `internal/issuemath/ridge_drift.go`, `internal/diagnostics/debug_tag_health.go` |
| Ridge debug endpoint | `internal/diagnostics/debug_ridge_score.go` |
| Negation pipeline | `internal/ai/openai.go`, `internal/ai/service.go`, `internal/issueenrichment/verify.go`, `internal/domain/tags.go` |
| Rank-1 fallback model | `internal/issuemath/factor_model.go` |
| Residual-cluster concept mining | `internal/issuemath/residual_clusters.go`, `internal/memories/synthesizer.go` |
| NMF themes (pure library) | `internal/issuethemes/themes.go` |
| Themes service (loadings adapter, revision cache, WP-203 identity) | `internal/themes/cache.go`, `internal/themes/loadings.go`, `internal/themes/identity.go` |
| Themes debug API (WP-204) | `internal/diagnostics/debug_themes.go`, `internal/api/debug.go` — `GET /debug/themes`, `GET /debug/themes/{id}`, `GET /debug/issues/{id}/themes` |
| Ranking consumers | `internal/map/search.go`, `internal/map/explore.go`, `internal/people/person_detail.go` |
| Map projection (Phase 6 target) | `internal/issuemath/projection.go` |
| Constants | `internal/scoring/constants.go` |
| Eval harness | `internal/matheval` ([math-eval.md](./math-eval.md)) |
