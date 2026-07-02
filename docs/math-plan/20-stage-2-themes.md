# Stage 2 — Themes to production

> Goal: turn `internal/issuethemes` — a complete, deterministic,
> zero-caller NMF library — into a stable, consumable corpus artifact with an
> API. Queue and status: [README.md](./README.md).

Entry criteria: WP-103 (decomposition cache) shipped — themes consume
full-corpus ridge loadings, and computing them per refresh without the cache
means redoing the exact work the cache exists to avoid.
Exit criteria: a revision-fresh set of themes with **stable identities**,
reachable over a debug API, with names a human would accept, and telemetry
that says whether K=8 and 50 iterations are right.

**What exists today (verified):** `internal/issuethemes/themes.go` — input
`V = max(0, f_i)` (all-nonpositive rows dropped); NNDSVD initialization
(gonum thin SVD; component 0 from `|u₀|√σ₀`, others from the dominant
sign-split; deterministic mean-based fallback); Lee–Seung multiplicative
updates (H then W); fixed 50 iterations, **no convergence check**; H rows
L1-normalized with mass pushed into W; themes ordered by descending W-column
mass, tie-broken by top tag name; top-5 tags; unit-normalized
embedding centroids; K = min(8, N, T). Bit-for-bit determinism is tested.
No persistence, no cache, no API, no UI, no callers.

---

## WP-201 — Corpus loadings source for themes

**Size:** S · **Depends on:** WP-103.

### Context

`issuethemes.Build` takes `[]IssueLoading{IssueID, Values}` + tag names +
tag embeddings. The decomposition cache (WP-103) holds exactly this: per-issue
`Loading` vectors over the tag index, plus the tag metadata used to build
them. The only glue missing is an adapter.

### Design

A function in the new themes service layer (not in `issuethemes`, which stays
a pure library): `loadingsFromDecomposition(cache result) []IssueLoading`,
mapping the cached loading vectors and the tag-name index. Reuse
`TagDataFromIssues` (`internal/issuemath/residual_clusters.go`) if the tag
name/embedding assembly isn't already exposed by the cache.

Decisions to make here, once, and document in the code:

- **Which issues participate:** match the drift sweep's rule — usable
  embedding AND at least one anchored tag. Issues with no analyzer opinion
  contribute `f` shaped purely by the unscored penalty; at the GCV λ (which
  pins unscored tags toward zero) they are near-zero rows that NMF drops
  anyway. Excluding them explicitly is clearer than relying on that.
- **Open vs all issues:** include closed issues. Themes describe the corpus,
  not the backlog; Stage 4 overlays filter by lifecycle at query time.
  (Gap analysis in particular *needs* historical issues.)

### Acceptance criteria

A service-layer function returns loadings for the current revision, unit
tested against a fixture decomposition, with the participation rule
documented and tested (no-embedding and no-anchor issues excluded).

---

## WP-202 — Revision-keyed theme cache

**Size:** S · **Depends on:** WP-201.

### Design

`internal/issuethemes` gains a sibling service package (or
`internal/themes`) with a `Cache` copied from the `ridgelambda` shape:
revision source, single-flight read-through compute, `(Result, ok)` return,
`(zero, false)` for degenerate corpora (fewer than ~2·K participating
issues — below that, "themes" are noise; constant named and documented).

**No DB persistence in this WP.** NMF at current scale is milliseconds once
loadings exist (the loadings are the expensive part, and WP-103 pays it).
In-memory-per-revision matches the two existing caches, and durable snapshots
are pulled in by exactly one future need — theme drift over time — which is
WP-405's decision, not this one's.

Compute chain per revision: decomposition cache → WP-201 adapter →
`issuethemes.Build` → WP-203 identity matching (once it exists) → cached
`Result`.

### Validation

Unit tests: revision bump recomputes; identical corpus → identical result
(determinism carries through the service); degenerate corpus → `(_, false)`;
concurrent `Current` calls compute once (`-race`).

### Acceptance criteria

`themesCache.Current(ctx)` returns revision-fresh themes; no caller computes
NMF directly; degradation shape matches the other caches.

### Outcome (WP-201+202 shipped together)

`internal/themes` service package (loadings adapter + cache); pure
`issuethemes` untouched. Plan corrections discovered: (a) raw `f` magnitudes
were NOT recoverable from the cached unit loadings — `RidgeVectors` gained a
`LoadingNorm` field (additive; ranking, equivalence, and baselines untouched)
so `raw f = Loading·LoadingNorm`; (b) the decomposition bundle carries
`TagNames`/`TagEmbeddings` (centered — the right space for WP-204 centroid
lookups), making `TagDataFromIssues` unnecessary, but lacks per-issue anchors,
so the cache takes a Store dep to read `TagScores` for the participation rule.
Floor: `minThemeParticipants = 16` (2·K). Determinism, single-flight, and
participation-rule tests all green under `-race`.

---

## WP-203 — Theme identity stability across refreshes

**Size:** M · **Depends on:** WP-202. **Blocks any UI or overlay work.**

### Why this is the critical WP of the stage

NMF has no identifiability guarantees. A refresh after a handful of issue
writes can permute components, split a theme, or merge two — and NNDSVD
initialization only makes the *same input* deterministic, not *similar
inputs* stable. Without identity matching, "theme 3" in a person's profile
silently becomes a different theme between page loads, and every Stage 4
overlay is built on sand. This is the map-Procrustes lesson applied to
themes: we already learned it once when layouts re-scrambled on refresh
(whitepaper §3.3); pay the cost before the UI exists, not after users notice.

### Design

Match new themes to previous themes by H-row similarity, carry stable IDs:

- The cache keeps the previous `Result` (previous revision's H, plus its
  assigned theme IDs). On recompute, build the K_new × K_old cosine matrix
  between L1-normalized H rows. K ≤ ~10, so optimal assignment is trivial —
  greedy is probably fine, Hungarian is 30 lines and removes the doubt; use
  Hungarian.
- A matched pair above `themeMatchThreshold` (start 0.6, constant, revisit
  with WP-206 telemetry) inherits the old theme's stable ID. Unmatched new
  rows mint fresh IDs (monotonic counter, never reused). Unmatched old IDs
  are reported as `retired` in the result — consumers decide what to show.
- Stable IDs are strings (`theme-<counter>`), decoupled from NMF component
  index and from display order.
- **Cold start / process restart:** in-memory previous-result means identity
  resets on restart. Acceptable for the debug tier (WP-204); becomes
  unacceptable when Stage 4 ships user-visible profiles — at that point,
  persist the tiny identity state (theme ID → H row, ~K×T floats) rather than
  full results. Note this explicitly as the *second* trigger for persistence
  (after WP-405), and leave it out of this WP.
- Emit match diagnostics with the result: per-theme match score, count
  minted/retired. This is the stability telemetry WP-206 aggregates.

### Steps

1. Hungarian matcher over H-row cosines, pure function, exhaustively unit
   tested (permutation → all matched at ~1.0; split theme → one match + one
   mint; shrunk corpus → retire).
2. Thread previous-result through the cache recompute; attach stable IDs and
   diagnostics to `Result`.
3. A soak test in the service tests: apply a sequence of small corpus
   mutations to a fixture, assert IDs persist across ≥ 10 refreshes with no
   spurious mints.

### Acceptance criteria

Theme IDs survive small corpus mutations; permutation alone can never change
an ID; mint/retire events are observable in the result; thresholds are named
constants with rationale comments.

### Outcome (shipped)

Potentials-based O(n³) Hungarian in-package; rows keyed by tag NAME over the
union (catalog changes align correctly); threshold 0.6 inclusive, boundary
asserted. Soak: 6 themes × 12 mutation refreshes → zero churn; deleting a
theme's tag+issues retires exactly that ID. Two behaviors beyond the spec,
both documented in code: matching operates on the top-5 tags exposed by
`issuethemes.Theme.Tags` (full H not exported; WP-206 remediation path noted),
and identity state survives degenerate `(zero,false)` revisions so a
temporarily-shrunk corpus resumes rather than churns.

---

## WP-204 — Themes debug API

**Size:** S · **Depends on:** WP-203.

### Design

Debug-tier first, exactly like every other math surface
(`internal/diagnostics`, registered in `internal/api/server.go` beside the
existing `/debug/*` routes):

- `GET /api/v1/debug/themes` — theme list: stable ID, weight (W-column
  mass share), top tags with loadings, centroid-nearest issues (top ~5 by
  cosine of centroid vs centered issue embedding), match diagnostics from the
  last refresh.
- `GET /api/v1/debug/themes/{id}` — one theme: full H row (nonzero tag
  loadings), top issues by W weight, centroid neighbors.
- `GET /api/v1/debug/issues/{id}/themes` — one issue's theme distribution
  (W row, keyed by stable ID).

Promotion out of `/debug` happens in Stage 4, when there is a product
consumer — not before, and only after WP-203's soak criteria have held on the
dev corpus for a while (define: no unexplained mint/retire churn over normal
usage).

### Validation

Handler tests with a fixture cache; manual pass on a seeded dev corpus:
`docker compose up -d`, seed, hit all three endpoints, sanity-read the
themes — do the top tags cohere? This qualitative read is part of the WP, not
optional: if K=8 themes on the dev corpus read as garbage, stop and open the
K/preprocessing question (WP-206) before building anything on top.

### Acceptance criteria

Three endpoints, tested, documented in the API docs alongside the other debug
endpoints; a recorded qualitative read of dev-corpus themes in the PR
description.

### Outcome (shipped)

Shipped as designed with two scope adjustments, both because there was no
live seeded dev server available in this environment (debug tier, no state
mutation permitted):

- **Qualitative read done on the matheval fixture corpus, not a seeded dev
  corpus.** `internal/themes/qualitative_test.go` runs the real themes cache
  over `matheval.GenerateCorpus()` (48 issues, 16 tags in 8 correlated
  domain pairs) and logs the theme list. Result: 8/8 themes cleanly recovered
  a real domain group (money, identity, documents, mobile, data+performance,
  discovery, and communications split cleanly into notifications/email) —
  not garbage, so no WP-206 stop-and-report was triggered. The dev-corpus
  manual pass from the Validation section above is still open and belongs to
  whoever seeds a long-lived dev server.
- **Centroid-nearest issues moved from the list endpoint to the detail
  endpoint only.** `GET /debug/themes` returns stable ID, weight share, top
  tags, and match diagnostics; `GET /debug/themes/{id}` adds centroid-top
  issues and top issues by W weight. Every issue's centroid similarity
  against every theme is O(themes × issues) — cheap enough per-theme on
  request, wasteful to compute for every theme on every list call. The full H
  row was not exposed either (`GET /debug/themes/{id}` surfaces the same
  top-5 tags as the list, with the limitation documented in the response's
  `tagsNote` field) — issuethemes does not export more than top-5, and this
  WP does not touch issuethemes (that's the WP-206 remediation path WP-203
  already flagged).

Plan correction: `internal/themes.Result` gained a `Means` field
(`issuemath.CorpusMeans`, additive) so the detail endpoint can center a raw
issue embedding into the same space as a theme's centroid — the decomposition
cache already computes these means, this just carries the value through
rather than re-deriving them. `internal/themes` also gained an exported
`ClassifyParticipation` (loadings.go) so `/debug/issues/{id}/themes` can
classify one issue's participation status without re-running the whole-corpus
adapter or duplicating the participation rule.

---

## WP-205 — Theme labeling

**Size:** S–M · **Depends on:** WP-204.

### Context

Top-5 tags are a serviceable fallback label ("auth, session, token, login,
expiry") but not a name. The repo already has the exact pattern needed:
residual-cluster concept mining names clusters of issues via
`ConceptProfiler.ProposeConceptFromCluster` (`internal/ai`), primed with the
project concept frame, with a graceful fallback when the LLM is unavailable.

### Design

- A `ThemeLabeler` surface in `internal/ai` (or a parameterization of the
  existing concept-profiler call): input = top tags with loadings + top
  centroid-neighbor issue titles + the project overview/concept frame; output
  = short name + one-sentence description.
- Labels are generated **per stable theme ID, on mint** — not per refresh.
  A theme that keeps its ID keeps its label (that is the point of WP-203).
  Relabel only when the matched H row drifts far from the row at labeling
  time (cosine below a relabel threshold), and keep the old label in the
  response as `previousLabel` so the UI can show continuity.
- Labels are the first genuinely persistence-needing theme state (they cost
  an LLM call — recomputing on restart is wasteful and can rename things).
  A tiny `theme_labels` table (stable ID, label, description, H-row-at-label,
  created_at) is acceptable scope here — it is an append-mostly side table,
  not the theme-snapshot persistence WP-405 debates. Alternatively defer the
  table and accept restart-relabeling at debug tier; decide by whether Stage
  4 timing is close.
- Stub provider behavior: fall back to top-tag join. Never block theme
  computation on labeling — labels attach asynchronously if needed.

### Acceptance criteria

Every theme in the debug API carries a name; names survive refreshes and
restarts (if the table was built) or the deferral is documented; LLM-off path
tested.

### Outcome (shipped)

`ThemeLabeler` in `internal/ai` (OpenAI + deterministic stub top-tag join),
bridged into `internal/themes` via a local `Labeler` interface with the
frame-assembling adapter at the composition root — themes stays decoupled
from `internal/ai`. Lifecycle: fallback label attached synchronously on mint,
LLM name arrives via a background job (mutex never held across the call;
generation-stale writebacks discarded); relabel below top-tag cosine 0.5
(`themeRelabelThreshold`, deliberately under the 0.6 identity threshold) with
`previousLabel` retained. **Persistence deferred**: in-memory store, restart
relabels, table lands with the Stage-4 identity persistence. Relabel decision
tested as a pure function (the 0.5–0.6 band is too narrow for a robust NMF
fixture).

---

## WP-206 — NMF convergence, K, and quality telemetry

**Size:** S–M · **Depends on:** WP-202 (can run parallel to 203–205).

### Context

Three hand-set choices in the factorizer, currently justified by nothing but
scale: fixed 50 MU iterations with no convergence criterion; K = 8; and
Frobenius-objective MU as the algorithm. All fine at 48-issue fixture scale;
unmeasured at real scale.

### Design

1. **Early stop:** track the relative Frobenius objective improvement per
   iteration; stop when < ε (1e-4) or at the iteration cap (raise cap to 200
   once early-stop exists — the cap becomes a backstop, not the terminator).
   Determinism is preserved (same input → same stopping point).
2. **Telemetry per refresh**, logged and attached to the cached result:
   final reconstruction error `‖V−WH‖_F/‖V‖_F`, iterations used, per-theme
   match scores from WP-203, mint/retire counts.
3. **K experiment harness** (a test-tier sweep, matheval-style, not
   production behavior): for K in 4..12 on a fixture corpus and on the dev
   corpus snapshot, record reconstruction-error elbow and identity-stability
   under a standard mutation sequence. Output: a table in this document and a
   decision — keep K=8, change the default, or (only if the data demands it)
   make K adaptive. Adaptive K is *not* pre-authorized: it interacts badly
   with identity stability (K changes force mints/retires), so the bar is
   high.
4. **Algorithm note:** MU convergence is slow near optima; HALS converges
   faster. Do NOT implement HALS now — record the pointer and the trigger
   (refresh telemetry showing iteration-cap hits or refresh latency that
   matters).

### Acceptance criteria

Early stop shipped with a determinism test; telemetry visible in the debug
API; the K table exists in this document with a dated decision; HALS trigger
documented.

### Outcome (shipped; K decision dated 2026-07-02)

Early stop: relative Frobenius improvement < 1e-4 terminates the MU loop, cap
raised to 200 as a backstop; `issuethemes.Result` carries `Iterations` and
`ReconstructionError`; determinism and monotone-objective tests included.
Notable: the old fixed-50 count was *under-converged* — the fixtures converge
at 58 (matheval, K=8) and 114 (soak) iterations, so early-stop+200 improved
factorization quality, not just cost. `themes.Result.Telemetry` (recon error,
iterations, mint/retire counts, mean inherited match score) is logged per
refresh and surfaced on `/debug/themes`. The cache gained an optional
`ThemeCount` override for experiments.

K sweep (opt-in `go test ./internal/themes/ -run ThemeCountSweep -themesweep -v`),
churn = mints+retires across 12 small deterministic mutations:

matheval fixture corpus (48 issues, 16 tags, true structure ≈ 8 groups):

| K | reconErr | iterations | churn/12 |
|---|----------|------------|----------|
| 4 | 0.7086 | 15 | 4 |
| 5 | 0.6452 | 15 | 0 |
| 6 | 0.5957 | 15 | 4 |
| 7 | 0.5452 | 19 | 0 |
| 8 | 0.4829 | 58 | 0 |
| 9 | 0.4254 | 102 | 12 |
| 10 | 0.3843 | 103 | 12 |
| 11 | 0.3427 | 107 | 8 |
| 12 | 0.3187 | 81 | 26 |

soak corpus (6 orthogonal groups × 8 issues, 6 tags; K clamps at 6):

| K | reconErr | iterations | churn/12 |
|---|----------|------------|----------|
| 4 | 0.5733 | 10 | 2 |
| 5 | 0.4060 | 20 | 4 |
| 6 | 0.0015 | 114 | 0 |
| 7 | 0.0015 | 114 | 0 |
| 8 | 0.0015 | 114 | 0 |

**Decision: keep K = 8.** Reconstruction error falls monotonically with K (it
must), so the elbow alone is a weak criterion; identity stability is the
discriminating signal, and it cliffs immediately past the corpus's true
structure count — K=9..12 churn 8–26 events per 12 refreshes vs zero at
K=7–8. Adaptive K remains unauthorized: the sweep directly demonstrates the
failure mode (K above structure ⇒ ghost themes that reshuffle every refresh).
Revisit only with real-corpus telemetry showing recon error stuck high AND
zero churn at K=8. **HALS trigger: not near firing** — max observed 114
iterations vs the 200 cap, no cap hits, refresh cost is milliseconds.

---

## Stage-2 exit checklist

- [ ] Themes reachable at `/debug/themes` with stable IDs and human names.
- [ ] Soak: no unexplained identity churn on the dev corpus across routine
      mutations (WP-203 criteria held during WP-204/205 development).
- [ ] Telemetry answering "is K=8 right, does 50-iteration MU converge".
- [ ] math-evolution.md Part I §3.8 rewritten from "built, unwired" to the
      as-built service description; Part II Track A marked delivered.
