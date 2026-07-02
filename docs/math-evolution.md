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
                  Similarity                     Themes (built,               Diagnostics
                  wF·cos(f_A,f_B)                unwired): NMF on             R² = 1−‖e−Tᵀf‖²/‖e‖²
                + wR·cos(res_A,res_B)            F⁺ = max(0, f)               DriftCosine(f, r)
                  default in search /            → W, H, centroids           → tag-health sweep
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
  `rhs = T e_i + Λ_i r_i`, one LU solve. O(K²) per issue; microseconds at
  K ≈ 200.
- The solver reuses scratch buffers and is therefore **not concurrency-safe**
  — currently fine (single-threaded per request), flagged in Track D.
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

Per grid point, each sampled issue costs a Cholesky factorization plus a
trace computed by materializing `A⁻¹·Gram` — the compute hot spot (Track D).
A grid point where `D − df ≤ 0` for any sample is rejected outright: GCV is
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

### 3.8 The themes factorizer (built, unwired)

`internal/issuethemes` is a complete, deterministic NMF library with **zero
production callers** — the backend math slice of Phase 4, awaiting integration
(Track A):

- Input `V = F⁺`: per-issue ridge loadings rectified to `max(0, f_i)`; rows
  with no positive mass dropped.
- **Initialization:** NNDSVD (Boutsidis–Gallopoulos) via thin SVD — component 0
  from `|u₀|√σ₀`, later components from the dominant sign-split of each
  singular vector pair; a deterministic mean-based seed fills degenerate
  components. No randomness anywhere; determinism is tested bit-for-bit.
- **Updates:** Lee–Seung multiplicative updates (Frobenius objective), a fixed
  50 iterations, **no convergence criterion** — a known simplification
  (§12).
- **Post-processing:** each theme's tag distribution `H_k` is L1-normalized
  (mass pushed into `W`), themes ordered by total `W` mass, top-5 tags per
  theme, and a unit-normalized centroid `v_k = unit(Σ_t H_kt · tag_embedding_t)`
  for labeling and search anchoring.
- **K:** default 8, clamped to `min(K, N, T)`. No data-driven selection yet.

## 4. Evidence: what the numbers say (and don't)

The evaluation harness (`internal/matheval`, [math-eval.md](./math-eval.md))
is a synthetic corpus: 16 tags, 48 issues, 32 fully-judged queries, embedding
dim 24, NDCG@8 / Recall@8.

| Measurement | NDCG@8 | Recall@8 | Where |
|---|---|---|---|
| Pre-centering baseline | 0.823 | 0.855 | whitepaper §2.0 |
| Rank-1, centered (committed golden baseline) | 0.8645 | 0.9117 | `matheval/testdata/baseline.json` |
| Rank-1, similarity-only (shadow harness) | 0.874 | 0.928 | `ridge_shadow_test.go` |
| Ridge, fixed λ_unscored = 0.05 (overfit, R² ~0.90) | 0.867 | — | shadow harness — *regression* |
| Ridge, GCV λ_unscored ≈ 3.0 (R² ~0.80), tag-space | **0.933** | **0.964** | shadow harness |
| Full-path A/B (ridge vs rank-1 through the whole pipeline) | +0.065 | +0.047 | one logged shadow run — recorded nowhere in code |

Honest caveats, all of which are Track D work:

1. **The committed golden baseline still guards the rank-1 path.** The default
   flip lives in the API layer (the handler injects `WithRidgeSimilarity` from
   the λ cache); the matheval golden run calls `SearchFromQueryWithTags` with
   no options — so the numbers that gate regressions are rank-1 numbers, and
   **the shipped production default has no regression guard.**
2. **The shadow numbers are documentation, not assertions.** The ridge shadow
   comparison is opt-in (`-ridge` flag) and log-only, and the full-path A/B
   deltas are not written down anywhere in the harness — they survive only as
   a run log cited in this document. WP-101 re-runs and pins them.
3. **The fixture corpus structurally favors the tag story.** Its embeddings are
   generated as relevance-weighted tag sums plus anisotropy plus noise — so the
   ridge win over rank-1 on fixtures is a floor argument, not proof on real
   embeddings. The harness flags this itself.
4. **Only search is harness-validated.** Explore and person recommendations
   inherit the identical model and blend code but have no eval of their own.

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
| NMF "~50 iterations to convergence" | Fixed 50 iterations, no convergence check | Simplification; acceptable at current scale, revisit with telemetry (§12). |

Documentation debt created by the flip (tracked in Track D): the comment header
over the ridge constants (`internal/scoring/constants.go`) still says
"debug/shadow only — not wired into ranking"; whitepaper §10.3 still describes
anchored ridge as shadow-mode while §2's status header correctly says it is the
default. Both are stale text, not stale code.

## 6. Status board

| Phase | Status | Where |
|---|---|---|
| **Phase 0** — foundations | **Done (reshaped).** Σ tightening dropped in favor of corpus-mean centering; drift shipped as `DriftCosine`. | `internal/issuemath/centering.go`, `ridgescore.go` |
| **Phase 1** — signed relevance | **Shipped end-to-end.** Analyzer negation with evidence gate, verifier negations (anti-alignment 0.5 / dominance 0.25), 0.7 cap honored on every path, JSONB persistence. Emitting: `analyzer-negation`, `verifier-dominance`. Dead constants: `dismiss`, `cooccurrence`. | `internal/ai/*`, `internal/issueenrichment/verify.go`, `internal/domain/tags.go` |
| **Phase 2** — ridge shadow | **Shipped.** Per-request anchored ridge behind `GET /debug/issues/{id}/ridge`, signed anchor, diagonal penalties. | `internal/issuemath/ridgescore.go`, `internal/diagnostics/debug_ridge_score.go` |
| **Phase 3** — default flip | **Shipped.** GCV λ revision-cached (`internal/ridgelambda`, stride-sampled ≤2000, centered with corpus means); search, explore, and person recommendations default to ridge tag-space blend with rank-1 fallback; downstream modifiers untouched. Caveats: only search is harness-validated; golden baseline still guards rank-1 (§4). | `internal/issuemath/ridge_decomposition.go`, `ridge_gcv.go`, `internal/ridgelambda/`, `internal/map/search.go`, `explore.go`, `internal/people/person_detail.go` |
| **Phase 3.5** — drift consumers | **Shipped (beyond original plan).** Tag-health sweep at fixed loose λ; curation detector uses drift as primary mis-tagging signal; rank-1 residual clusters demoted to uncovered-concept mining (propose-only). | `internal/diagnostics/debug_tag_health.go`, `internal/curation/detect.go`, `internal/issuemath/residual_clusters.go`, `internal/memories/synthesizer.go` |
| **Phase 4** — themes | **Backend math built, fully unwired.** Deterministic NNDSVD + Lee–Seung NMF over `F⁺` with tests. No persistence, no refresh, no API, no UI, no caller. | `internal/issuethemes/` |
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

Estimate: 1–2 days of math, but gated on A3 soak time and the matheval map
metric. Do not schedule it earlier than that.

## 10. Track D — Hardening and honesty

The audit-driven track. Ordered by how much risk each item retires.

**D1. Put the production default under the golden baseline.** The matheval
golden run must exercise the ridge path (inject the GCV λ the way the API layer
does) and commit new baseline numbers; keep a second rank-1 baseline entry to
guard the fallback. Until this lands, the shipped ranker has no regression
guard — this is the single most important item in Part II.

**D2. A real-embedding fixture.** The synthetic corpus generates embeddings as
tag sums, structurally favoring ridge. Capture a small anonymized real corpus
(or real-model embeddings over fixture texts) with judged queries. Until then,
treat every fixture delta as a floor argument, not a measurement.

**D3. Eval coverage for explore and person recommendation.** Both inherit the
ridge blend with zero harness coverage. Explore needs a seeded-neighbor
judgment set; person-fit needs a small assignment-history fixture.

**D4. Revision-keyed decomposition cache** (gates A1). Cache
`ComputeRidgeDecomposition` output per corpus revision so search, explore,
people, tag-health, and themes stop re-solving per request. Prior perf work
already amortized the Gram; the per-request N-solve loop is the remaining
recompute. Invalidation is the same revision counter every other cache uses.
Make the solver concurrency story explicit at the same time (per-goroutine
solver or pooled scratch) — the shared-scratch solver is a latent race if
anything ever parallelizes.

**D5. GCV cost and consistency.** Two smaller solver items: (a) the trace
`tr(A⁻¹·Gram)` materializes a full K×K product per sample per grid point —
compute it via Cholesky column solves accumulating only the diagonal, or accept
it with a comment (fine at K ≈ 200, not at K ≈ 1000); (b) GCV selects λ on a
Cholesky path while the ranker solves via LU — numerically close but not
identical; unify on one factorization when touching either.

**D6. Negation completion or cleanup.** Decide the two dead provenances:
build a real per-issue tag-dismiss affordance emitting `dismiss` negations
(high precision, low volume — still the best future source), and either wire
co-occurrence `ImplicitNegative` into background `r⁻` under the §3.2
operational rules or delete the `cooccurrence` constant. (The tag-relevance
copy paths were audited for negation-field loss and are correct — whole-struct
assignment carries the value-type provenance fields; only the pointer/slice
fields need the explicit re-copies they already have.)

**D7. Stale-text sweep.** `internal/scoring/constants.go` ridge header
("debug/shadow only"); whitepaper §10.3 shadow-mode paragraph; any remaining
`‖f − r‖` drift references. One small PR.

**D8. Standardize `TagDrift.Delta`.** Raw per-tag deltas are not comparable
across tags (Λ varies per tag). Standardize against each tag's corpus delta
distribution before ranking spurious/missing candidates — improves tag-health
precision cheaply.

**D9. Inherited open weak spots** (whitepaper §10.2, still open, unowned by
any phase): Ledoit–Wolf (or explicitly-documented heuristic) covariance
shrinkage; the asymmetric authority/hubness slopes (0.25 vs 0.15); adaptive
`k` in tag specificity. None block A–C; schedule opportunistically.

## 11. Sequencing

```
D1 (baseline guards ridge)  ──────────────►  ship first, standalone
D4 (decomposition cache) ──► A1..A4 themes ──► A5..A6 ──► B1..B5 overlays
                                   │                          │
                                   └── A3 soak + map metric ──┴──► C (map on W)
D2, D3, D5–D9: parallel, opportunistic; D2 before believing any new fixture delta.
```

Rules of the road, unchanged: every item is a small stacked PR; math changes
ship dark or debug-tier first; anything user-visible gets a shadow window and
an eval metric before the flip — that discipline is what made the ridge flip
safe, and it is the template.

## 12. Open mathematical questions

Genuinely unresolved, kept visible so they get decided by evidence rather than
by default:

1. **Theme count K.** Fixed at 8. Candidates once A6 telemetry exists:
   reconstruction-error elbow, H-row stability across revisions, or both.
2. **NMF convergence.** Fixed 50 MU iterations, no objective tracking. Add the
   early-stop in A6; the open question is whether MU is even the right
   algorithm at larger N (HALS converges faster) — irrelevant at current scale.
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
| NMF themes (unwired) | `internal/issuethemes/themes.go` |
| Ranking consumers | `internal/map/search.go`, `internal/map/explore.go`, `internal/people/person_detail.go` |
| Map projection (Phase 6 target) | `internal/issuemath/projection.go` |
| Constants | `internal/scoring/constants.go` |
| Eval harness | `internal/matheval` ([math-eval.md](./math-eval.md)) |
