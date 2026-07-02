# Stage 1 — Harden the shipped core

> Goal: the math that already runs in production becomes guarded, cached,
> comparable, and honestly documented. No new product surface in this stage.
> Queue and status: [README.md](./README.md).

Entry criteria: none — this is the front of the queue.
Exit criteria: the ridge default path is under the golden baseline; the ridge
decomposition is computed once per corpus revision instead of once per
request; drift deltas are comparable across tags; no stale text claims ridge
is shadow-only.

---

## WP-101 — Put the ridge default under the golden baseline

**Size:** S–M · **Depends on:** nothing · **Priority: highest in the program.**

### Context (verified)

- Production search/explore/people default to the ridge tag-space blend: the
  API layer injects `WithRidgeSimilarity(λ)` when `ridgelambda.Cache.Current`
  returns ok (`internal/search/search_issues.go` `ridgeOption`,
  `internal/search/search_unified.go`, `internal/api/mapview/explore_issue.go`,
  `internal/people/person_detail.go`).
- The matheval golden run calls `SearchFromQueryWithTags` with **no options**
  (`internal/matheval/eval_test.go`, `runSearchEval`), so the committed
  baseline (`testdata/baseline.json`: NDCG@8 0.8645 / Recall@8 0.9117) guards
  the **rank-1 fallback**, not the shipped default.
- The ridge comparison exists only as `TestRidgeShadowComparison`
  (`ridge_shadow_test.go`) — opt-in via a `-ridge` flag, `t.Skip` otherwise,
  and log-only. Its comment block records similarity-only numbers (rank-1
  0.874/0.928 vs GCV ridge 0.933/0.964); the full-path A/B deltas
  (+0.065/+0.047 cited in math-evolution §4) were logged once and recorded
  nowhere.

### Design

Two baselines, both asserted on every test run:

1. **`ridge` baseline (the default path).** The eval mirrors production
   injection: select λ via `SelectRidgeLambdaGCV` over the fixture corpus
   (same call shape as `ridgelambda.Cache.compute` — centered embeddings,
   `RidgeAnchorLambdaScored` held fixed), then run the search eval with
   `WithRidgeSimilarity(λ)`. Do **not** hardcode the λ value in the baseline;
   record it in the baseline file for observability but re-derive it each run
   so grid or GCV changes surface as metric deltas, not silent drift.
2. **`rank1` baseline (the fallback path).** The existing no-options run,
   kept — the fallback is real behavior for small corpora and must not rot.

Baseline file grows from one entry to a keyed structure, e.g.
`{"rank1": {...}, "ridge": {..., "gcvLambdaUnscored": 3.0}}`. The
`assertNoRegression` tolerance machinery applies to both.

### Steps

1. Extract the λ-selection preamble from `TestRidgeShadowComparison` into a
   shared helper in the matheval package (fixture corpus → centered
   embeddings → GCV λ), so the golden path and the shadow test share it.
2. Add the ridge-path eval run alongside the rank-1 run in `eval_test.go`;
   thread the option through `runSearchEval`'s opts parameter (already
   accepts opts).
3. Restructure `testdata/baseline.json` to the keyed shape; regenerate via
   the harness's update flow; commit both baselines with the numbers in the
   PR description.
4. Keep `TestRidgeShadowComparison` as the exploratory tool (it also sweeps
   the fixed-λ overfit case), but add a comment pointing at the now-guarded
   baseline as the source of truth.
5. Update math-evolution §4 caveat 1 (baseline guards rank-1 only) — it
   becomes false when this ships. Pin the re-measured full-path deltas there.

### Validation

`go test ./internal/matheval/...` (both baselines asserted); then a manual
tamper test: perturb `RidgeAnchorLambdaScored` locally and confirm the ridge
baseline fails while rank-1 passes. `mise run check` before submit.

### Outcome (shipped)

Both paths guarded on every test run. Measured: rank1 0.8658/0.9117 (the old
0.8645 was stale-within-tolerance), ridge **0.9309/0.9586** at grid-stable
GCV λ=3.0 — full-path delta +0.0651/+0.0469, matching the historical shadow
run. Tamper finding: λ_scored 0.5→0.8 *improves* ranking and passes; the
guard was proven at λ_scored=3.0 (ridge fails on factorWeight/R² drift, rank1
green). Determinism held across runs; the pin-λ fallback was not needed.

### Acceptance criteria

- A change that regresses the ridge blend fails CI without any opt-in flag.
- Baseline file contains both entries plus the recorded GCV λ.
- math-evolution §4 updated; the "no regression guard" caveat removed.

### Risks / notes

- The fixture favors tag-space methods (embeddings are tag sums); the ridge
  baseline will look flattering. That is fine for a *regression guard* — it
  detects change, not absolute quality. Absolute honesty is WP-301's job.
- If GCV on the 48-issue fixture proves grid-unstable across platforms
  (float differences flipping between grid points), pin the λ in the baseline
  file and assert the *selection* separately with a tolerance of one grid
  step. Decide only if observed.

---

## WP-102 — Stale-text and misleading-comment sweep

**Size:** XS · **Depends on:** nothing · can ship any time.

### Context (verified)

Three places still describe the pre-flip world, one comment misdescribes a
statistic:

1. `internal/scoring/constants.go:143` — header over the ridge λ constants
   reads "Anchored ridge regression (debug/shadow only — not wired into
   ranking)". False since the Phase 3c flip: those constants back the
   production ranking path.
2. `docs/whitepaper.md` §10.3 (~L671–676) — "Anchored ridge regression exists
   in shadow mode… not persisted and not consumed by ranking." Contradicts the
   same document's §2 status header and the code.
3. `internal/issuemath/factor_model.go` (~L38) — struct comment calls the
   aggregate "mean R²"; the code computes a pooled variance ratio
   (`Σ projVar / Σ totalVar` — the `validCount` divisor cancels).
4. Any remaining `‖f − r‖` drift references outside the deviations ledger
   (grep docs and comments; `DriftCosine` is canonical).

### Steps

One PR: fix all four; grep for "shadow" in comments under `internal/` and
"drift" in `docs/` to catch stragglers. While in `constants.go`, replace the
header with one that states the two-λ regime explicitly (ranking uses GCV
`λ_unscored`; drift/tag-health deliberately uses the fixed loose constant).

### Validation

`mise run compile` (comment/doc-only). Read the whitepaper §10.3 diff
carefully — rewrite it to describe the shipped state (default flip, fallback,
two-λ regime) rather than deleting it.

### Acceptance criteria

No text in the repo claims ridge is shadow-only; no comment claims the
aggregate is a mean; the constants header teaches the two-λ regime.

---

## WP-103 — Revision-keyed ridge decomposition cache + solver concurrency

**Size:** M–L · **Depends on:** WP-101 (semantic changes need the ridge guard).

### Context (verified)

- Only the scalar GCV λ is cached (`internal/ridgelambda/cache.go`, revision
  keyed, stride-sampled ≤ 2000, centered with the shared revision-cached
  corpus means). The decomposition itself — Gram build O(K²·D) plus one
  O(K²) solve per issue — is recomputed **per request** in search
  (`internal/map/search.go` ~L260), explore (`explore.go` ~L125), people
  (`person_detail.go` ~L175), and the tag-health sweep
  (`debug_tag_health.go` ~L131).
- `ridgeSolver` (`ridge_decomposition.go` ~L232–303) reuses mutable scratch
  buffers (`gram`, `projection`, `rhs`, `f`, `out`) — **not concurrency-safe**,
  and nothing documents or guards that.
- Prior perf work already amortized allocation hot spots (GCV/ridge
  allocation, redundant-Gram wins); the remaining recompute is the per-request
  N-solve loop and Gram copy. Two adjacent recomputes to sweep in the same
  pass: search builds the tag covariance twice per query (once inside
  `ComputeFactorDecomposition`, once directly), and curation's duplicate
  detection (`internal/curation/detect.go` `DetectDuplicates`) runs one
  explore per seed — each currently recomputing the decomposition, which is
  why it needed a seed cap; it becomes a direct beneficiary of this cache.

### Design

A `ridgedecomp.Cache` following the `ridgelambda` pattern exactly (revision
source, read-through compute, `(value, ok)` degradation), holding the
**full-corpus** decomposition at the GCV λ:

- Compute over all issues with usable embeddings (centered with the shared
  means), not a stride sample — the decomposition is the product artifact,
  not an estimate of a scalar.
- **Semantic watch-out:** today each surface computes the decomposition over
  its own item set (search corpus load, explore neighborhood, people's open
  issues), so `AggregateR2`/blend weights are *set-relative*. A shared
  full-corpus cache makes weights corpus-relative — a deliberate improvement
  (stabler weights) but a behavior change that must go through the WP-101
  baseline: expect small metric movement; re-baseline consciously, in its own
  commit, with the delta in the PR description.
- Consumers switch from `ComputeRidgeDecomposition(items…)` to
  `cache.Current(ctx)` + `VectorsFor(id)` per candidate. Candidates missing
  from the cache (created since the revision bump? no — any write bumps the
  revision; missing means no embedding) keep the existing
  pure-semantic-at-full-weight behavior.
- The tag-health sweep keeps its own λ regime (fixed loose unscored penalty)
  and therefore does NOT read this cache — it recomputes with its own Λ, or a
  second cache entry keyed `(revision, regime)` if profiling says it matters.
  Do not unify the regimes (conventions §6).
- **Concurrency:** make the solver safe under the cache — either construct a
  fresh solver per compute (compute is single-flight per revision anyway) and
  document the invariant, or give `solve` per-call scratch. Add a
  `-race`-exercised test that hits `Current` from multiple goroutines during
  a revision bump.

### Steps

1. Package `internal/ridgedecomp` (mirror `ridgelambda`'s structure, deps:
   store, tags, revisions, centering, ridgelambda for the λ).
2. Single-flight the compute (the `ridgelambda` cache's mutex pattern), so a
   burst of first requests after a bump computes once.
3. Swap search → cache; run matheval; re-baseline if weights moved; commit
   with the measured delta.
4. Swap explore and people (same stack, separate branches).
5. Solver concurrency test + doc comment on the invariant.
6. Leave `ComputeRidgeDecomposition` exported — matheval and any set-scoped
   analysis still use it directly.

### Validation

matheval both baselines (WP-101); `go test -race ./internal/ridgedecomp/...`;
a benchmark comparing p50 search latency before/after on a seeded dev corpus
(`docker compose up -d`, seed, hit `/api/v1/search`) — the point of the WP is
that this drops.

### Acceptance criteria

- Search/explore/people no longer call `ComputeRidgeDecomposition` per
  request; one compute per corpus revision, race-clean.
- Baseline deltas (if any) measured and committed deliberately.
- Fallback behavior unchanged: cache `(nil, false)` → rank-1 path, covered by
  a small-corpus test.

### Risks

- Memory: N × (K + 2D) floats per revision (loading + recon + residual). At
  10k issues, K=200, D=1536: ~250 MB if all three vectors are kept. **Decide
  what to keep:** ranking needs `Loading` (K floats) and `Residual` (D floats)
  per issue; `Reconstruction` is derivable and only the debug endpoint wants
  it. Keeping loading+residual halves it; storing residuals as float32 halves
  again. Size this against the actual deployment corpus before building.
- Set-relative → corpus-relative weight change could shift explore feel more
  than search (explore sets are small and local). Watch WP-302's explore eval
  once it exists; if explore regresses, explore can keep set-relative weights
  computed from cached per-issue R² — cheap, since the vectors are cached.

---

## WP-104 — Standardize per-tag drift deltas

**Size:** S · **Depends on:** nothing (touches diagnostics only).

### Context (verified)

- Tag-health ranks spurious/missing tag candidates by raw
  `TagDrift.Delta = f_k − r_k` with a flat floor
  (`DefaultDriftTagDeltaFloor = 0.3`, `debug_tag_health.go:27`).
- Λ varies per tag (scored 0.5 vs unscored 0.05 in the drift regime), so raw
  deltas are not comparable across tags: a loosely-penalized unscored tag
  swings wider than an anchored one by construction. The debug endpoint's own
  comments acknowledge the non-comparability.

### Design

Per-tag standardization within the sweep: `ComputeCorpusDrift` already visits
every issue, so accumulate per-tag delta mean/variance in the same pass, then
report `zDelta = (delta − mean_k) / max(std_k, ε)` alongside the raw delta.
Rank candidates by |zDelta|; keep the raw floor as a secondary guard so a tag
with near-zero corpus variance can't produce huge z-scores from noise
(require BOTH |delta| ≥ floor AND |zDelta| ≥ z-threshold, z-threshold start
at 2.0). Tags with < ~20 observations fall back to the raw-delta rule —
z-scores from tiny samples are worse than none.

### Steps

1. Extend `IssueDrift`/`TagDrift` with `ZDelta` (computed in
   `ComputeCorpusDrift`; nil/omitted when the tag's sample is too small).
2. Tag-health handler ranks by the combined rule; response includes both
   values; curation detector (`internal/curation/detect.go`) consumes the new
   ordering unchanged (it reads the handler's output).
3. Unit test with a synthetic corpus where a high-variance unscored tag
   currently out-ranks a genuinely drifted scored tag, asserting the z-rule
   flips the order.

### Validation

`go test ./internal/issuemath/... ./internal/diagnostics/...`; eyeball the
`/debug/tag-health` output on the dev corpus before/after (the flagged issues
should get more plausible, not just different).

### Acceptance criteria

Candidate ranking is z-based with documented small-sample and low-variance
guards; the curation detector's input ordering reflects it; constants named
in `debug_tag_health.go` with the rationale.

---

## WP-105 — GCV solver cost and factorization consistency

**Size:** S–M · **Depends on:** WP-101 (touching GCV needs the guard).

### Context (verified)

- `SelectRidgeLambdaGCV` computes `df = tr((TTᵀ+Λ)⁻¹TTᵀ)` by
  `chol.SolveTo(&xinv, gram)` then `mat.Trace` — materializing a full K×K
  product per sample per grid point (`ridge_gcv.go` ~L158–161). At the GCV
  sample bound (2000 issues × 7 grid points) and K in the hundreds this is
  the dominant cost of a λ recompute.
- Two factorizations solve "the same" system: GCV uses Cholesky; the ranking
  solver uses LU (`SolveVec`, `ridge_decomposition.go` ~L296). Numerically
  close, not identical — λ is selected on a slightly different path than the
  one that ranks.

### Design

- **Trace:** compute `tr(A⁻¹G)` without the full product. With Cholesky
  `A = LLᵀ`: `tr(A⁻¹G) = Σ_j (A⁻¹g_j)_j` — solve per column but accumulate
  only the diagonal element, or equivalently `‖L⁻¹G^{1/2}‖²_F` when G's
  factor is available (it is: `G = TTᵀ` gives `G^{1/2}`-free form
  `tr(A⁻¹TTᵀ) = ‖L⁻¹T‖²_F` with `L⁻¹T` computed by one triangular solve
  against the K×D matrix — O(K²D) once per grid point, **independent of
  sample count** since A varies only via Λ… note Λ is per-issue, so A is
  per-sample; the win is avoiding the K×K materialization, not the loop).
  Benchmark first; if the measured λ-recompute time at production scale is
  already < ~100ms, take option (b): a comment documenting the accepted cost
  and a bail-out `if K > threshold` telemetry warning.
- **Consistency:** unify both paths on Cholesky (SPD is guaranteed —
  `TTᵀ + Λ` with Λ > 0). One factorization type, one numerical story, and
  Cholesky is ~2× cheaper than LU. Guarded by the WP-101 baseline (expect
  zero metric movement; assert that).

### Steps

1. `go test -bench` a realistic-shape benchmark (K=200/500, D=1536,
   samples=2000) for the current GCV; record numbers in the PR.
2. Implement the cheaper trace; assert equality with the old computation to
   1e-9 on random SPD fixtures.
3. Switch `ridgeSolver.solve` to Cholesky; run matheval; expect bit-level
   deltas only.
4. Delete the now-false "different factorization" caveat from
   math-evolution Part I §3.4 (D5 note).

### Acceptance criteria

Measured speedup (or a measured, documented decision not to bother); one
factorization type across GCV and ranking; baselines unchanged within
tolerance.

### Risks

Low. Numerical: Cholesky requires SPD — Λ has strictly positive entries on
every path today (0.05 floor); add a defensive check that falls back to LU
with a telemetry counter rather than panicking.
