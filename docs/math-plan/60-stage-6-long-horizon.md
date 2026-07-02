# Stage 6 — Long horizon

> Not a sequenced stage: a curated backlog of math work that is real but not
> on the critical path. Ordered roughly by value. Pick items up once Stages
> 1–2 are done; each is independently schedulable unless noted.
> Queue: [README.md](./README.md).

---

## WP-601 — Per-issue tag dismiss affordance → `dismiss` negations

**Size:** M (mostly product surface).

### Context (verified)

`NegationProvenanceDismiss` is a dead constant — defined in
`internal/domain/tags.go`, produced nowhere. The existing `/tags/dismiss`
endpoint is unrelated: it dismisses *tag-merge suggestions* into
`dismissed_tag_merges` (migration 000011) and never touches
`TagRelevance.Negation`. There is no way for a user to say "this tag is wrong
on this issue" and have the math see it.

### Design

- API: `POST /api/v1/issues/{id}/tags/{tag}/dismiss` → writes
  `Negation` on that issue's `TagRelevance` (magnitude: a named constant,
  start 0.6 — high-precision human signal, deliberately above the verifier
  magnitudes and below the 0.7 cap), provenance `dismiss`,
  `NegationReason` "user dismissed", no evidence ranges (the act is the
  evidence). Undo endpoint restores the prior state (store the pre-dismiss
  negation, if any, to make undo exact).
- Enrichment interplay — the design decision that needs care: a re-analysis
  must not resurrect a dismissed tag. Rule: `dismiss` provenance is sticky —
  `applyAnalyzerNegations`-adjacent merge logic treats a `dismiss` negation
  as a floor that analyzer output cannot lower (analyzer can *raise* the
  negation, nothing can erase it except undo). Test this against the
  re-enrichment path specifically.
- UI: a small affordance on the tag chip (the `r⁻` "ruled out" display
  pattern already exists for negated tags).
- This is the highest-precision negative-signal source the design
  anticipated; it also generates the labeled data WP-603 needs.

### Acceptance criteria

Dismiss + undo round-trip tested; re-enrichment cannot resurrect; the math
layer sees the signed anchor immediately (signedAnchor already consumes any
`Negation` — verify with a ridge debug-endpoint before/after in the test).

---

## WP-602 — Co-occurrence negative evidence: wire or retire

**Size:** S–M · **Depends on:** WP-601 (establishes the dismiss floor UX and
the merge rules a background source must respect).

### Context (verified)

`internal/tagcooccurrence` computes directed `ImplicitNegative` via shrunk
lift with sparse-corpus guards; it feeds search-time anti-correlation
penalties (`internal/search/search_issues.go` → `WithAntiCorrelators`) and
issue tag-set coherence in `internal/issuexray`, and deliberately excludes
synthetic negation rows. `NegationProvenanceCooccurrence` is a dead constant.

### The decision this WP exists to make

Structural anti-correlation ("issues tagged `ios` are rarely `windows`") is
**population** knowledge; `r⁻` is a **per-issue** claim. Auto-writing
population knowledge onto individual issues violates the §3.2 evidence bar
(math-evolution) — statistically-usually-true is not textual evidence.

Recommendation, to be confirmed or overturned with data: **do not wire
co-occurrence into `r⁻`.** Instead:

- Keep it where it is (search penalties, coherence).
- Optionally surface it as a *suggested dismiss* in the WP-601 UI ("`windows`
  rarely co-occurs with this issue's other tags — dismiss?"), which converts
  population signal into human-confirmed per-issue signal at exactly the
  right trust boundary.
- Delete the `cooccurrence` provenance constant if the suggestion route is
  taken (the resulting negation is provenance `dismiss`), or keep it if a
  direct background writer is ever justified by measured analyzer blind
  spots.

Whatever the outcome: record it in math-evolution §5 (deviations ledger) and
close the dangling constant either way. A dead constant is a promise someone
will eventually keep by accident.

---

## WP-603 — Negative-evidence magnitude calibration

**Size:** M · **Depends on:** WP-601 shipped and accumulating data.

### Context

Every negation magnitude is hand-set: analyzer cap 0.7, verifier
anti-alignment 0.5, verifier dominance 0.25 (calibrated only to reproduce the
old ×0.75 shrink), dismiss (WP-601's 0.6). The design principle (§3.2:
negative evidence expensive, capped, provenance-tracked) is right; the
*numbers* have no empirical footing.

### Design

Calibrate against outcomes the system already observes:

- **Ground truth events:** a user dismisses a tag the verifier had negated
  (agreement), a user re-adds/uses a tag that carried a negation
  (contradiction), re-enrichment confirms or reverses a negation, drift
  direction at negated coordinates.
- **Per-provenance precision:** P(negation was right | provenance), where
  "right" = never contradicted within a window. If verifier-dominance
  negations at 0.25 are contradicted materially more often than
  analyzer-negations at their cap, the magnitudes should reflect that — the
  magnitude *is* a confidence claim, so make it track measured confidence.
- Deliverable is a **report and a proposal**, not an auto-tuner: a periodic
  job (or matheval-style offline pass over the event history) producing the
  per-provenance table, and a human-reviewed constants change with the table
  in the PR. Do not close the loop automatically — the volumes are small and
  the failure mode (self-reinforcing negation) is ugly.

### Acceptance criteria

The precision table exists and is reproducible by command; at least one
constants review has happened with it; the method documented in the
whitepaper's verifier section.

---

## WP-604 — Covariance shrinkage: Ledoit–Wolf or documented heuristic

**Size:** S–M · **Depends on:** WP-303 (the map metric is how you'd see the
effect). Whitepaper §10.2 item 2, open since the audit.

### Context (verified)

`buildTagCovariance` (`internal/issuemath/projection.go`) shrinks
`Σ_shrunk = α·Σ + (1−α)·I` with `α = clamp(1 − mean(offdiag²), 0.1, 1.0)` —
a heuristic the whitepaper itself calls "wrong direction in spirit" vs
Ledoit–Wolf (more correlation → *more* shrinkage toward identity would be
LW's move under estimation noise; this rule does the opposite because its
purpose is smearing for the map, not covariance estimation).

### Design

Small honest study, three arms, judged by the WP-303 suite (map) plus the
ranking baselines (Σ also feeds the rank-1 fallback's `ê = Tᵀ(Σ·r)`):

1. Status quo (baseline).
2. Ledoit–Wolf shrinkage intensity (closed-form, cheap at T×T).
3. No shrinkage (`α = 1`) — the null hypothesis that the knob doesn't matter
   at current catalog sizes.

Likely outcome, admitted upfront: at T ≈ tens-to-hundreds of well-embedded
tags the differences are small, and the WP resolves as "keep the heuristic,
rewrite the comment to state its real purpose (map smearing, not estimation),
close §10.2-2 as documented-by-choice". That is a fine outcome — the item's
cost is that it currently *looks* like an oversight.

---

## WP-605 — Authority/hubness slopes + adaptive specificity k

**Size:** S · Whitepaper §10.2 items 8 and 10, batched.

### Context (verified)

- `authority = min(1, dupCount·0.25)`, `hubness = min(1, linkCount·0.15)` —
  asymmetric hand-set slopes, no recorded rationale.
- Tag specificity uses kNN with hard-coded `k = min(8, n−1)`; percentile
  ranks are catalog-relative and go stale as the catalog grows.

### Design

- Slopes: sweep both on the (post-WP-301) fixtures via the existing
  sweep-test pattern; either the metrics move (pick the argmax
  and record it) or they don't (unify both slopes to one named constant with
  a comment saying the value is insensitive — asymmetry with no reason is
  the smell, not the number).
- Specificity k: switch to `min(8, ⌈√n⌉)` (the whitepaper's own suggestion)
  guarded by the tag-quality consumers' tests; add a staleness note — the
  percentile re-rank already happens per catalog refresh, verify that's true
  under the current refresh triggers and document where.

---

## WP-606 — GCV in the K ≥ D regime

**Size:** M · No dependencies; priority rises if a small-dimension embedding
model or a very large tag catalog is ever considered.

### Context (verified)

`SelectRidgeLambdaGCV` rejects any grid point where a sample's
`D − df ≤ 0`; when every point is rejected it returns `(0, false)` and the
system silently runs rank-1 forever. Production today: D = 1536, K = catalog
size (≪ D) — safe. But the failure is *silent by design*, and the fixture
regime (D = 24) is closer to the cliff than production is.

### Design

1. **Observability first:** a telemetry counter/log when GCV returns not-ok
   with the reason (too small vs degenerate-df) — the difference between
   "corpus is tiny" and "we are in a regime our math can't handle" should be
   visible without reading code.
2. **Then the math, only if a real corpus approaches the regime:** candidate
   fixes, in order of preference — df computed against the effective rank of
   T (SVD-truncated) rather than D; restricted GCV on a tag subset;
   or a documented hard floor on λ_unscored replacing selection when df
   saturates. Do not build these speculatively; the WP's shippable unit is
   the observability plus a written decision rule for when to build the rest.

---

## WP-607 — Outcome supervision spike

**Size:** L (research spike, timeboxed) · **Depends on:** Stage 4 shipped and
accumulating theme-tagged history.

### Framing

Everything in this program is unsupervised structure. The obvious next
question — do themes *predict* anything (resolution time, duplicate rate,
reopen rate, staleness)? — needs labels, which lifecycle events already
provide for free.

### Design (timebox: one week, deliverable is a memo)

- Assemble the design matrix from history: per issue, theme weights `W_i` at
  creation (via WP-405's projection-onto-current-H machinery) + lifecycle
  outcome labels.
- Simple models only (regularized linear/logistic per outcome) — the point is
  measuring signal existence, not modeling. K ≈ 8 predictors means even small
  corpora can answer.
- Honest nulls: permutation tests, and a tags-only baseline (does `W` beat
  raw tag relevance as a predictor? If not, themes add no *predictive* value
  over tags — still fine, their value is structural — but say so).
- The memo decides whether a product signal ("issues in this theme
  historically take 3× longer") is worth building. No product code in the
  spike.

---

## WP-608 — Curation & memory math whitepaper

**Size:** M (writing + audit, no behavior change).

### Context

The curation ("librarian") and durable-memory surfaces run real scoring math
— duplicate-pair detection, quiet/redundant memory detection, reinforcement
candidates, residual-cluster concept mining thresholds — none of it
documented in either the whitepaper or math-evolution (both confirmed by
audit; the gap is called out in math-evolution §13). Undocumented math rots
into folklore; this program's own history (the "R² means what exactly?"
confusion that motivated the ridge work) is the cautionary tale.

### Design

Audit-then-document, the same method that produced math-evolution Part I:

1. Fan out over `internal/curation`, `internal/memories` (synthesizer,
   reinforcement, redundancy pairing), and the memory-search scoring;
   extract every formula, threshold, and guard with file:line anchors.
2. Write `docs/curation-memory-math.md` in the whitepaper's voice: formulas,
   consumers, critical notes (hand-set constants flagged), what's sound /
   what isn't.
3. File the weak spots it surfaces as new Stage 6 WPs — this document is how
   the program discovers its own next backlog.

### Acceptance criteria

The doc exists, verified against code the way this plan was (adversarial
fact-check pass); math-evolution §13's "different document" pointer updated
to point at it.

---

## Parking lot (not WPs yet — promote when real)

- **HALS for NMF** — trigger: WP-206 telemetry shows iteration-cap hits or
  refresh latency that matters.
- **Blend-weight discontinuity at the 5-issue fallback boundary**
  (math-evolution §12.5) — trigger: any report of small-corpus ranking
  weirdness; the fix is a smooth ramp between fallback weights and the
  clamped aggregate.
- **Query-aware blend weights** (whitepaper §2.5 notes weights are
  corpus-wide; tag-name queries get a fixed nudge) — trigger: WP-301/302
  evidence that per-query adaptation pays.
- **Edge quality** — edges remain uncentered-cosine thresholds with no eval
  coverage (whitepaper §3.2); trigger: user reports or a Stage 5 follow-up
  once positions are themed and edge/position dissonance becomes visible.
- **Search error on hash-fallback embeddings** (whitepaper §9's remaining
  open item — fallback is instrumented but search still silently degrades) —
  trigger: fallback counters showing nonzero rates in real deployments.
