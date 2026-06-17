# Sortit Math Evolution: Toward a Quantitative Planning Layer

> Scope: this document describes the **next** version of Sortit's math layer,
> as developed in design discussion. It is forward-looking. The current
> implementation is documented in [whitepaper.md](./whitepaper.md); this doc
> describes where the model is heading and the rationale for each change.
>
> The goal stated by the team: *"a quantitative overlay on project planning
> that goes beyond embeddings and RAG."* Embeddings give you semantic
> retrieval; tags give you interpretation; this layer is what turns those
> into per-person, per-iteration, per-theme quantitative signals usable for
> planning, coverage analysis, and prioritization.

---

## 1. Recap: what we're evolving away from

The current model (see [whitepaper §2](./whitepaper.md#2-the-factor--residual-decomposition))
calls itself a "factor model" but is, mathematically, a **rank-1 per-issue
projection**:

```
ê_i      = Tᵀ (Σ_shrunk · r_i)        // synthesized direction
û_i      = ê_i / ||ê_i||
factor_i = (e_i · û_i) · û_i           // 1-D projection, zeroed when e·û ≤ 0
residual = e_i − factor_i
```

After normalization, `cos(factor_A, factor_B) ≈ cos(ê_A, ê_B)` — a function
purely of tag loadings and tag embeddings. The issue embedding contributes
**only an alignment gate** to factor similarity: an anti-aligned embedding
(`e · û ≤ 0`) zeroes the factor rather than flipping it (whitepaper §2.3),
and otherwise the embedding's degree of support is discarded. All the
embedding signal lives in the residual side, weighted by `w_R`.

That's a defensible heuristic, but it's not a factor model. It also leaves
significant signal on the floor:

- No per-tag attribution of how text explains tags.
- No corpus-level "themes" — factors are per-issue lines, not shared axes.
- No "AI vs. embedding" disagreement signal.
- No mechanism for negative evidence ("this is *not* a Safari bug").

The plan below addresses all four.

---

## 2. Constraints that shape the design

These are non-negotiables established in design discussion:

1. **Positive loadings are the default.** Tags express "this issue is
   about X" — a non-negative claim. Negative loadings can exist but require
   explicit generation; they do not arise naturally from AI analysis.

2. **Factor model is an analogy, not a strict commitment.** We are not
   bound to textbook factor analysis with mixed-sign loadings, latent
   factor identifiability, or maximum-likelihood estimation. The math
   serves the product.

3. **Regression is fine.** Specifically: regression over an *unconstrained*
   relevance vector that has both positive and negative entries. Once
   negative loadings exist, ridge regression with closed-form solution
   replaces NNLS (which we'd need only if positivity were enforced).

4. **Complexity is acceptable when it pays off in planning overlays.**
   The product goal is structured, quantitative signals for planning —
   not just better search ranking.

5. **The AI's `r_i` remains load-bearing.** It is the analyzer's
   interpretation of the issue and what users see in the UI. The math
   layer refines it, but it is not replaced wholesale by a regression
   estimate.

---

## 3. The new model in one diagram

```
                                       ┌─────────────────────────────────┐
  Issue text  ──►  AI analyzer  ─────► │  r_i⁺ ∈ [0,1]^T  positive tags │
                       │               │  r_i⁻ ∈ [0,1]^T  negated tags  │
                       │               │  evidence ranges per tag       │
                       │               └─────────────────────────────────┘
                       │                              │
                       │                              ▼
                       │            r_i = r_i⁺ − r_i⁻  ∈ ℝ^T  (signed)
                       │                              │
                       ▼                              ▼
                   embedding e_i ∈ ℝ^D     ┌────────────────────────────┐
                       │                   │ Anchored ridge regression  │
                       └─────────────────► │ f_i = (TTᵀ + λI)⁻¹(Te + λr)│
                                           │     ∈ ℝ^T                  │
                                           └────────────────────────────┘
                                                       │
                       ┌───────────────────────────────┼───────────────────────────────┐
                       ▼                               ▼                               ▼
                  Similarity                       Planning                         Diagnostics
                  cos(f_A, f_B)                    overlays on f_i,                  R² = 1−‖e−Tᵀf‖²/‖e‖²
                  cos(e_A, e_B)                    aggregated per                    ‖f_i − r_i‖ (drift)
                                                   person / iteration /              negation hit rate
                                                   project area
```

---

## 4. Signed relevance: `r_i = r_i⁺ − r_i⁻`

Today the analyzer emits a single non-negative tag-relevance vector per
issue. Going forward, it emits **two**:

- `r_i⁺ ∈ [0,1]^T`: tags the issue is positively about (today's behavior).
- `r_i⁻ ∈ [0,1]^T`: tags explicitly refuted, with source-text evidence.

The math layer consumes `r_i = r_i⁺ − r_i⁻ ∈ [-1, 1]^T`. The UI displays
`r_i⁺` as familiar tag chips; `r_i⁻` shows up as a separate "ruled out"
affordance.

This pattern preserves backward compatibility: any consumer that ignores
`r_i⁻` sees today's behavior. New consumers benefit from the signed signal.

### 4.1 Sources of `r_i⁻`

| Source | Priority | Notes |
|---|---|---|
| **Analyzer negation detection** | Primary | Explicit refutation in source text — see §6 |
| **User dismiss actions** | Secondary | Already tracked via `/tags/dismiss`. High precision, low volume. |
| **Verifier down-rank made explicit** | Free | Today's `DownRank` verdict implicitly says "tag X doesn't fit." Make it an `r_i⁻` entry instead. |
| **Co-occurrence anti-correlation** | Background | Once corpus ≥ a few hundred well-tagged issues, derive structural anti-correlations from data. |
| **Refinement contradiction** | Deferred | When a refinement post contradicts an existing tag. Requires analyzer comparison. |
| **Mutual-exclusion taxonomy** | Optional | Declare `{bug, feature, improvement, cleanup}` as mutually exclusive in the catalog. |

### 4.2 Operational rules for negative emission

- **Confidence floor.** Only emit `r_ik⁻ > 0` for a tag when evidence is
  strong (explicit negation, verifier dominance, or user dismiss).
- **Magnitude cap.** `r_ik⁻ ≤ 0.7`. Leaves room for the math to disagree.
- **Provenance required.** Every `r_ik⁻ > 0` carries a source identifier
  (`analyzer-negation`, `dismiss`, `verifier-dominance`, `cooccurrence`).

False negatives are worse than false positives — they actively contradict.
The bar for emitting negative signal must stay higher than the bar for
emitting positive signal.

---

## 5. Anchored ridge regression: `f_i`

With signed `r_i ∈ [-1, 1]^T`, the embedding-grounded factor scores are
the closed-form solution:

```
f_i = argmin_{f}  ||e_i − Tᵀ f||² + λ ||f − r_i||²
    = (T Tᵀ + λ I)⁻¹ (T e_i + λ r_i)
```

Interpretation:

- `||e_i − Tᵀ f||²`: how well a tag-weighted combination of tag embeddings
  reconstructs the issue embedding. The "data fit" term.
- `λ ||f − r_i||²`: how close `f` stays to the AI's signed judgment. The
  "AI prior" term.
- `λ`: trust knob. `λ → ∞` recovers `f = r_i`; `λ → 0` is pure least-squares
  reconstruction.

### 5.1 Why ridge, not NNLS

The earlier design assumed positivity and pointed to NNLS (Lawson-Hanson
active-set, iterative, `O(K²)` per iteration). Once we allow signed `r_i`,
the non-negativity constraint goes away and the problem becomes a single
quadratic with closed-form solution. **One matrix solve, no iteration, no
constraint handling.**

### 5.2 Computational shape

- Precompute `G = T Tᵀ + λI ∈ ℝ^(K×K)` once per catalog refresh. (With
  `T ∈ ℝ^(K×D)` — rows are tag embeddings — the K×K Gram matrix is `T Tᵀ`,
  not `TᵀT`; this is what `internal/issuemath/ridgescore.go` implements.)
- Per-issue solve: `f_i = G⁻¹ (T e_i + λ r_i)`. Cost: `O(K²)` matrix-vector
  + `O(K)` add + one already-cached inverse.
- At `K ≈ 200`, each issue is microseconds.
- Total corpus refresh: trivial. Can run on every mutation; almost
  certainly cheaper than the current PCA pipeline.

### 5.3 What `f_i` gives us that `r_i` doesn't

| Question | Today (`r_i`) | With `f_i` |
|---|---|---|
| "Does the embedding agree with the tagging?" | Indirectly via rank-1 R² | Directly via `‖f − r‖` |
| "Which tags does the text geometry actually support?" | Not derivable | Read off `f_i` components |
| "What's the AI's confidence vs. text geometry's confidence?" | Conflated | Separated as `r_i` vs `f_i` |
| "Is this tag spurious?" | Verifier flag, post-hoc | `f_ik` much smaller than `r_ik⁺` |
| "Is this tag missing?" | No mechanism | `f_ik` substantially larger than `r_ik⁺` |
| "Multi-dimensional per-issue explanation" | No | Yes — all `K` components are interpretable |

The last row is the unlock. `f_i ∈ ℝ^T` is a proper multi-dimensional
positive-or-signed loading vector that can be aggregated, projected,
clustered, and reasoned about.

### 5.4 New diagnostic: AI/embedding drift

```
drift(i) = ‖f_i − r_i‖
```

High `drift(i)` means the AI's tagging is significantly different from
what the embedding geometry supports. This is **a new signal that exists
in neither embeddings nor RAG**.

Use cases:
- **Issue review queue**: surface high-drift issues for human verification.
- **Tag-catalog hygiene**: tags with high mean drift across many issues
  may be poorly defined or have weak embeddings.
- **Analyzer quality monitoring**: aggregate drift over time as a
  regression metric for prompt or model changes.

---

## 6. Analyzer prompt change for negation

The analyzer today returns:

```json
{
  "tags": [
    { "name": "safari", "relevance": 0.8, "evidence": ["fails in Safari only"] },
    { "name": "bug", "relevance": 0.7, "evidence": ["doesn't work as expected"] }
  ],
  "embedding": [...]
}
```

The proposed change adds a parallel `negated_tags` field:

```json
{
  "tags": [
    { "name": "safari", "relevance": 0.8, "evidence": ["fails in Safari only"] },
    { "name": "bug", "relevance": 0.7, "evidence": ["doesn't work as expected"] }
  ],
  "negated_tags": [
    {
      "name": "regression",
      "confidence": 0.85,
      "evidence": ["this is not a regression; it never worked"]
    }
  ],
  "embedding": [...]
}
```

### 6.1 Prompt addendum

Concretely, the section added to the existing analyzer prompt:

```
In addition to identifying tags that apply to this issue, identify tags from
the candidate set that the issue text EXPLICITLY REFUTES. A tag is refuted
only when the text contains direct negation of it — for example:

  - "This is not a regression"
  - "Doesn't affect Safari"
  - "Working as designed, not a bug"
  - "Customer confirmed this is intentional behavior"

Do NOT mark a tag as negated merely because:
  - The text doesn't mention that tag
  - You suspect the tag might not apply
  - Another tag fits better

Negation requires textual evidence. For each negated tag, return:
  - name: the canonical tag name from the candidate set
  - confidence: how clearly the text refutes the tag, in [0, 1]
  - evidence: an array of direct quotes from the text containing the negation

Output negated_tags as an empty array if no tags are explicitly refuted.
The negation evidence will be cross-verified against the source text; do not
fabricate quotes.
```

### 6.2 Verifier cross-check

The same `resolveEvidenceRanges` machinery the verifier uses today (in
[`internal/issueenrichment/verify.go`](../internal/issueenrichment/verify.go))
applies to `negated_tags`. A negation claim whose evidence quote cannot be
located in the source text is **discarded**, not kept-with-flag. The bar
for negative signal is strictly higher than for positive.

### 6.3 Persistence shape

`r_i⁻` lives alongside `r_i⁺` on the issue:

```go
type TagRelevance struct {
    Tag         string
    Relevance   float64           // r⁺[tag], in [0, 1]
    Negation    *float64          // r⁻[tag], in [0, 1], nil if absent
    Provenance  string            // "analyzer-positive" | "analyzer-negation" |
                                  // "dismiss" | "verifier-dominance" | "cooccurrence"
    Evidence    []EvidenceRange   // source-text ranges, includes negation evidence
    // ... existing verifier metadata
}
```

The signed value `r_i = r_i⁺ − r_i⁻` is derived at math-layer entry; storage
keeps both components for auditability.

---

## 7. Corpus-level themes via NMF on `F`

`f_i` is per-issue T-dimensional. For planning at the corpus level, we
need a *small* set of human-readable themes.

Run NMF (clamped to non-negative parts of `f_i`, since NMF requires
non-negative inputs) on the issue-by-tag positive-loading matrix:

```
F⁺ ∈ ℝ_+^(N×T)  where F⁺[i, k] = max(0, f_i[k])
F⁺ ≈ W · H      with W ∈ ℝ_+^(N×K), H ∈ ℝ_+^(K×T)
```

Pick `K ∈ [5, 10]` initially. Each row of `H` describes a theme by its
tag loadings; each row of `W` gives an issue's theme distribution.

For each theme `k`:

```
v_k = Σ_t H_kt · tag_embedding_t     (normalized to unit length)
```

`v_k ∈ ℝ^D` is the theme's centroid in embedding space, useful for
labeling and search anchoring.

### 7.1 NMF specifics

- **Initialization**: NNDSVD (non-negative double SVD). Deterministic; gives
  stable seeds across reruns.
- **Updates**: multiplicative updates of Lee-Seung. ~50 iterations to
  convergence at Sortit scale.
- **Cost**: `O(N · T · K)` per iteration. At N=10k, T=200, K=8 — milliseconds total.
- **Refresh**: trigger on the same revision-bump signal that invalidates
  the map projection. Themes drift slowly; refresh on every Nth mutation
  is fine.
- **K selection**: start fixed at K=8. Once telemetry exists, pick K by
  reconstruction-error elbow or by stability of `H` rows across reruns.

### 7.2 What themes are not

Themes are **not** factors in the textbook sense. They have no
identifiability guarantees, no causal interpretation, no statistical-test
backing. They are *structured summaries of co-occurrence patterns* derived
from the positive parts of `f_i`.

That's enough for planning overlays. It is not enough for hypothesis
testing or scientific claims about the corpus. Document this distinction
internally.

---

## 8. Planning overlays this enables

The point of all the math above is to enable structured, quantitative
project-planning signals. Each overlay below is computable from `f_i`, `W`,
and existing lifecycle primitives.

### 8.1 Per-person profile

```
profile(person) = mean of f_i over issues assigned to person
                = mean of W_i ∈ ℝ_+^K   (theme distribution)
```

Differs from today's person profile in that it uses `f_i` (embedding-validated)
instead of raw AI relevance, and aggregates in theme space `W` for the
high-level view.

### 8.2 Per-iteration coverage

```
coverage(iteration) = sum of W_i over the iteration's issues, normalized
```

Read off as: "this iteration is 40% theme-3, 25% theme-5, 15% theme-1, ..."
Coverage gaps become directly visible.

### 8.3 Theme drift across time

```
drift(t) = 1 − cos(mean W in window [t−Δ, t], mean W in window [t−2Δ, t−Δ])
```

How much has the team's focus shifted between two time windows? Useful for
"what changed since last quarter."

### 8.4 Gap analysis

```
gap_score(k) = (catalog mean specificity of top tags in theme k)
             × (1 − coverage(k) over recent issues)
             × (historical authority of issues in theme k)
```

Themes with high specificity, low recent coverage, and meaningful past
authority are likely underserved areas. Surfaces themes that *used to
matter* and have gone quiet.

### 8.5 Person-to-theme fit

```
fit(person, theme_k) = cos(profile(person), H_k)
```

For routing: which person's historical profile best matches a theme? Use
for recommendations, but not as automatic assignment — humans interpret
the score.

### 8.6 Issue-level drift signal (from §5.4)

```
drift(i) = ‖f_i − r_i‖
```

Surface as a debug column in the issue UI. Filter for review queue:
"issues where AI tagging substantially disagrees with embedding geometry."

### 8.7 Tag-catalog hygiene

```
catalog_drift(tag k) = mean over issues with r_ik⁺ > threshold of:
                          | f_ik − r_ik⁺ |
```

Tags whose loadings consistently differ from AI claims may have weak
embeddings (catalog issue), be over-applied (analyzer issue), or have
unclear definitions (taxonomy issue).

---

## 9. What changes in the codebase

Listed in dependency order. Each item is a small, reviewable PR.

| # | Change | Files |
|---|---|---|
| 1 | ~~Tighten `Σ` to non-negative via `max(0, cos)`~~ **Dropped.** Corpus-mean centering landed first (whitepaper §2.0; `internal/issuemath/centering.go`, applied to tag embeddings at the corpus-load boundary before `buildTagCovariance`). Centered cosines are legitimately signed — negative entries now encode genuine anti-correlation between tag directions, and clamping them to zero would destroy that signal. | — |
| 2 | Add `Negation` and `Provenance` to `TagRelevance` domain type | `internal/issues/*`, migrations |
| 3 | Analyzer prompt update; structured `negated_tags` parsing | `internal/ai/*`, `internal/issueenrichment/analyze_text.go` |
| 4 | Cross-verify negation evidence (reuse `resolveEvidenceRanges`) | `internal/issueenrichment/verify.go` |
| 5 | Verifier `DownRank` verdict emits `r⁻` instead of multiplying | `internal/issueenrichment/verify.go` |
| 6 | New `internal/issuefactors` package: ridge solver, `f_i` storage, refresh | new package |
| 7 | Replace `synthesizeFactorEmbedding` / `factor_model.go` consumers with `f_i`-based similarity | `internal/map/search.go`, `internal/map/explore.go` |
| 8 | NMF on `F⁺`; persist `W`, `H`, `v_k` | extends `internal/issuefactors` |
| 9 | Map projection switches from PCA-on-X' to PCA-on-W | `internal/issuemath/projection.go` |
| 10 | Planning-overlay endpoints (per-person, per-iteration, theme detail) | `internal/api`, `internal/people`, new `internal/themes` |
| 11 | Debug endpoints: drift per issue, catalog drift, theme stability | `internal/api/debug_*` |

### 9.1 Migration strategy

The signed relevance change is non-destructive: `r_i⁺` is today's data;
`r_i⁻` is new and starts empty. Every existing consumer that reads
`Relevance` continues to work.

The `f_i` layer can ship dark first — computed and persisted but not
consumed for ranking. Run shadow comparisons against the existing rank-1
similarity for a window before flipping.

The NMF layer ships behind a feature flag. UI surfaces (theme list, person
profile updates) gate on the flag.

The PCA-on-W map projection is the riskiest change to user-facing behavior.
Keep the old projection available behind a debug flag for comparison.

---

## 10. Staging

| Phase | Scope | Estimated work |
|---|---|---|
| **Phase 0** | Tighten `Σ` to non-negative. Add drift metric placeholder. | 1 day |
| **Phase 1** | Negation in analyzer; `r⁻` storage; verifier cross-check. No math consumer changes. | 2–3 days |
| **Phase 2** | Ridge regression for `f_i`. Shadow mode — compute and store, don't consume. | 2 days |
| **Phase 3** | Flip similarity blending to use `f_i`. Drop `factor_model.go`. Drift signal in debug UI. | 2 days |
| **Phase 4** | NMF on `F⁺`. Theme persistence, refresh worker, theme list page. | 3–4 days |
| **Phase 5** | Planning overlays: per-person, per-iteration coverage, gap analysis. UI work dominates. | 1–2 weeks |
| **Phase 6** | Map projection switches to PCA-on-W. | 1–2 days |

Phases 0–3 are the math core. Phase 4 unlocks the planning vision. Phases
5–6 are product polish that consumes the math.

### 10.1 Implementation status

What is actually in the tree, verified against the code:

| Phase | Status | Where |
|---|---|---|
| **Phase 0** | Σ tightening **dropped** (see §9 item 1 — corpus-mean centering made signed cosines legitimate). Drift metric shipped as a restricted cosine (`DriftCosine`, zero-zero components excluded) rather than the `‖f − r‖` norm. | `internal/issuemath/centering.go`, `internal/issuemath/ridgescore.go` |
| **Phase 1** | **Shipped end-to-end.** Analyzer emits `negated_tags` with evidence; verifier cross-checks quotes before applying; persisted as `Negation` / `NegationProvenance` / `NegationEvidence` / `NegationReason` on `TagRelevance`. Emitting provenances today: `analyzer-negation` and `verifier-dominance` (the §9 item 5 DownRank conversion). `dismiss` and `cooccurrence` provenances are defined but not yet emitted. | `internal/ai/openai.go`, `internal/ai/service.go` (`normalizeNegated`), `internal/issueenrichment/verify.go` (`applyAnalyzerNegations`), `internal/domain/tags.go` |
| **Phase 2** | **Shadow mode, on demand.** Anchored ridge with per-tag diagonal penalties (scored vs unscored anchors) computed per request behind `GET /api/v1/debug/issues/{id}/ridge`, anchored on signed `r⁺ − r⁻`. Not persisted, not consumed by ranking. | `internal/issuemath/ridgescore.go`, `internal/diagnostics/debug_ridge_score.go`, `internal/scoring/constants.go` (`RidgeAnchorLambda*`) |
| **Phase 3** | **3a measured (shadow); λ made data-driven.** `ComputeRidgeDecomposition` builds per-issue `f_i`, reconstruction `Tᵀf`, residual, and true R² = 1 − ‖e − Tᵀf‖²/‖e‖²; `RidgeBlend` offers tag-space and reconstruction-space similarity. The matheval shadow harness (`TestRidgeShadowComparison`, `-ridge`) compares both against rank-1. **Result:** the debug-endpoint default `λ_unscored=0.05` overfits (unscored tags soak up variance; R² inflated to ~0.90) and *regresses* ranking. The ranking penalty is not a transferable constant — it depends on tag-catalog conditioning and the K/D ratio — so `SelectRidgeLambdaGCV` derives it per corpus by generalized cross-validation (no labels, no held-out split). On the fixtures GCV picks `λ_unscored≈3.0`, landing **tag-space** ridge at NDCG 0.933 / Recall 0.964 vs rank-1's 0.874 / 0.928 (honest R² ~0.80). Tag-space beats reconstruction-space (settles the similarity-shape fork). **3b shipped (opt-in):** `WithRidgeSimilarity(λ)` swaps the rank-1 blend for the GCV-λ ridge tag-space blend in `SearchFromQueryWithTags`, keeping every downstream modifier identical and falling back to rank-1 on sub-threshold corpora. The full-path A/B (all modifiers) confirms the win survives the pipeline: NDCG +0.065 / Recall +0.047 vs rank-1. The default is still rank-1; 3c flips it and extends the same swap to explore, person recommendations, and the map projection, then retires `factor_model.go`. | `internal/issuemath/ridge_decomposition.go`, `internal/issuemath/ridge_gcv.go`, `internal/map/search.go`, `internal/matheval/ridge_shadow_test.go` |
| **Phase 4** | Not started — no NMF, no `internal/issuefactors` / themes package. | — |
| **Phase 5** | Not started. The existing person profile/correlation endpoints predate this design and aggregate `r_i`, not `f_i`. | — |
| **Phase 6** | Not started — the map still projects PCA-on-X′ (now with Procrustes alignment against the previous layout for orientation stability; see whitepaper §3.3). | `internal/issuemath/projection.go` |

---

## 11. What this is *not* solving

To stay honest, several things outside the scope of this design:

- **Constant calibration.** `λ`, `K`, NMF iteration count, drift thresholds
  — all of these are hyperparameters. They need an evaluation harness, not
  hand-tuning. See [whitepaper §10.2 item 9](./whitepaper.md#102-weak-spots-worth-tightening).
- **Outcome supervision.** Knowing which themes predict *resolution time*,
  *duplicate rates*, or *user effort* requires supervised regression on top
  of `W` against labeled outcomes. That's a follow-on, not part of this design.
- **Embedding model quality.** Garbage in, garbage out. If tag embeddings
  are noisy, `f_i` will be noisy. Improving tag-embedding generation is
  out of scope here but worth a separate review.
- **Hash-based embedding fallback.** Still exists in `runtime.go`. The
  signed-loading design doesn't change this. See [whitepaper §9](./whitepaper.md#9-the-embedding-fallback).

---

## 12. Why this is worth doing

Three things this layer gives you that pure embeddings + tags cannot:

1. **A real R² that means what it says.** `1 − ‖e − Tᵀf‖² / ‖e‖²` is the
   textbook variance-explained metric, not the rank-1 squared-cosine the
   current code labels "R²". When you report `R²` in a debug surface, it
   will mean what people expect it to mean.

2. **A drift signal — `‖f − r‖`.** This is *new information* derivable
   neither from embeddings nor from tags in isolation. It directly powers
   issue review queues, catalog hygiene metrics, and analyzer quality
   monitoring.

3. **Theme-level aggregation as a first-class object.** With `W` and `H`
   in hand, every planning question ("what is this iteration about?",
   "where are the coverage gaps?", "who works on what?") becomes a
   vector-space operation on objects the math defines. The product can
   build views on top of these without re-deriving structure each time.

The rank-1 model can't deliver any of these. The cost — one analyzer
prompt change, one new package, one shadow-mode swap — is small enough
that the question becomes "why not?" rather than "why?"

---

## 13. Appendix: notation diff vs. whitepaper

| Symbol | Whitepaper | This doc |
|---|---|---|
| `r_i` | `∈ [0,1]^T`, AI relevance | `r_i⁺ − r_i⁻ ∈ [-1, 1]^T`, signed |
| `r_i⁺` | — | `∈ [0,1]^T`, analyzer-positive tags |
| `r_i⁻` | — | `∈ [0,1]^T`, refuted tags |
| `ê_i` | `Tᵀ Σ r_i`, used for projection direction | Deprecated |
| `factor_i` | Rank-1 projection of `e_i` onto `û_i` | Deprecated |
| `residual_i` | `e_i − factor_i`, 1-D orthogonal complement | Deprecated |
| `f_i` | — | Ridge regression solution `(TTᵀ + λI)⁻¹(Te + λr)`, ℝ^T |
| `R²_i` | `1 − ‖residual_i‖² / ‖e_i‖²` (rank-1) | `1 − ‖e − Tᵀf‖² / ‖e‖²` (full rank) |
| `drift(i)` | — | `‖f_i − r_i‖` |
| `W` | — | NMF issue-by-theme score matrix `∈ ℝ_+^(N×K)` |
| `H` | — | NMF theme-by-tag loading matrix `∈ ℝ_+^(K×T)` |
| `v_k` | — | Theme `k` embedding centroid `Σ_t H_kt · tag_embedding_t` |
| `λ` | — | Ridge regularization weight; controls AI prior strength |
