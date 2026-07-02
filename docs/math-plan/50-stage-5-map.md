# Stage 5 — Map projection on themes

> Goal: switch map positions from PCA over the tag-relevance matrix to a
> projection of theme weights — the original Phase 6. Deliberately last: the
> map is muscle memory, and this is the program's riskiest user-facing change.
> Queue: [README.md](./README.md).

Entry criteria (both hard):

1. **WP-303 shipped** — the three map metrics (neighborhood preservation,
   cluster legibility, refresh stability) baselined on the current
   projection. Without them this stage cannot measure what it risks.
2. **WP-208 done** — theme identities stable on the dev corpus through
   normal usage (the dev-corpus soak and qualitative read, split out of
   Stage 2 with its own owner). Positions derived from themes inherit every
   identity wobble as layout wobble.

**What exists today (verified):** `ComputePositionsAligned`
(`internal/issuemath/projection.go`) — PCA over the N×T issue-tag relevance
matrix smeared through the shrunk tag covariance (`X·Σ`), per-issue quality
weights `max(0.1, contentConfidence·maturity)`, deterministic sign
convention, orthogonal Procrustes alignment (reflections allowed, ≥3 shared
issues) against the previous normalized layout, robust IQR normalization to
[0.05, 0.95]. Map **edges** are uncentered-embedding cosine thresholds,
independent of positions, and explicitly out of scope for this stage.

---

## WP-501 — PCA-on-W projection, dark, behind a debug flag

**Size:** M · **Depends on:** entry criteria above.

### Design

- **Input:** the theme cache's W matrix (stable-ID-ordered columns, so the
  input basis doesn't permute between refreshes — this is why WP-203 gates
  the stage), same per-issue quality weights as today.
- **Reduction:** K ≈ 8 → 2. Weighted PCA over W rows, reusing the existing
  eigendecomposition path — mathematically identical machinery to today with
  `X·Σ` replaced by `W`. Resist anything fancier (t-SNE/UMAP): they trade
  the determinism and Procrustes-alignability that the current stack is
  built on for prettier local structure, and refresh stability is the
  property users feel most.
- **Continuity chain preserved verbatim:** sign convention → Procrustes
  against previous layout → robust normalization. The Procrustes previous
  layout at flip time is the *old projection's* layout — the first themed
  layout aligns to what users currently see, which softens the transition.
- **Issues with no theme mass** (no positive loadings — dropped by NMF):
  fall back to their current-projection position when one exists, else
  centroid-of-neighbors placement; never (0,0) clumping. This set should be
  small (verified: near-zero rows are exactly the issues with no anchored
  tags); surface its size in the debug output.
- **Wiring:** a projection-source switch at the `ComputePositions` call
  boundary, selectable per request via a debug query parameter
  (`?projection=themes`) — not an env var, so the two layouts are directly
  comparable in one running system. Default remains the current projection.

### Steps

1. `ProjectThemePositions` alongside the existing function, sharing the
   weight/align/normalize helpers (extract them if entangled).
2. Debug parameter through the map endpoint; both layouts computable on
   demand.
3. Run the WP-303 metric suite against the themed layout on fixture + dev
   corpora. Record the three-metric comparison table **in this document**.
4. Qualitative pass: side-by-side dev-corpus screenshots, annotated — do the
   clusters read as the themes they are made of? (They should almost by
   construction; verify anyway.)

### Acceptance criteria

Both projections live simultaneously; metric table recorded; no change to
default behavior; fallback placement for themeless issues tested.

### Risks

- K=8 input to a 2-D PCA keeps at most 2 theme directions' variance —
  themes beyond the top few collapse onto the plane. If the neighborhood
  metric degrades vs today's T-dimensional input, consider projecting from
  `W` concatenated with a low-weight residual term, or accept that the map
  trades local nuance for legible macro-structure — but let the metrics and
  the side-by-side make that argument explicitly.
- Theme mints/retires move basis columns. Stable-ID ordering + Procrustes
  absorbs most of it; the WP-303 stability metric under the standard
  mutation sequence is the acceptance gate, same bar as today's projection.

---

## WP-502 — Map flip: soak, compare, switch default

**Size:** S · **Depends on:** WP-501.

### Design

The flip itself, kept boring:

1. **Soak window:** WP-501's debug parameter in real use on the dev/team
   corpus for a defined period (suggest: two weeks of routine usage), with
   the metric suite re-run on whatever mutations occurred naturally.
2. **Flip criteria, pre-committed here** (edit before the soak, not after):
   themed layout ≥ current layout on refresh stability; within tolerance on
   neighborhood preservation; strictly better on cluster legibility; no
   unexplained themeless-issue population growth; qualitative sign-off.
3. **Flip PR:** default switches to themes; the old projection stays
   reachable via the debug parameter for at least one release cycle;
   whitepaper §3 rewritten (the map section changes fundamentally —
   positions become "where your themes are", which is the product story this
   whole program was pointed at).
4. **Rollback:** the switch is a parameter default — reverting is one line.
   Say so in the PR to make the flip decision low-drama.

### Acceptance criteria

Pre-committed criteria met and recorded; default flipped; old projection
still reachable; whitepaper §3 + math-evolution Part I/II updated; the
Phase 6 row in the status board closed.

---

## Stage-5 exit = program exit (of the planned arc)

With Stage 5 shipped, every item in math-evolution Part II Tracks A–C is
as-built, and the remaining open surface is Stage 6's long tail. The
program's closing move: rewrite math-evolution.md one more time — Part II
shrinks to Stage 6 + open questions, Part I becomes the complete manifest —
and fold the durable lessons into the whitepaper's critical-notes sections.
