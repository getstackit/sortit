# Math Program: Execution Plan

> The sequenced work-package (WP) queue for the math evolution program.
> Strategy and rationale live in [../math-evolution.md](../math-evolution.md)
> (Part I: what shipped; Part II: the track-level plan). Current runtime
> behavior lives in [../whitepaper.md](../whitepaper.md). This directory is the
> **working plan**: pick the next `todo` WP in queue order, execute it per
> [00-conventions.md](./00-conventions.md), update its status row here.
>
> Every factual claim in these documents was verified against the code on
> 2026-07-02 (two independent adversarial fact-check passes). File:line
> references may drift; the claims themselves were true at that date.

## How to work this plan

1. Read [00-conventions.md](./00-conventions.md) once — it defines the
   execution discipline (stacked PRs, ship-dark-then-flip, cache patterns,
   definition of done).
2. Take the first `todo` WP whose dependencies are `shipped`. WPs within a
   stage are ordered; stages are ordered; the queue below is the single source
   of truth for "what's next".
3. Each WP section in the stage documents contains: context (verified
   file:line anchors), design, implementation steps, validation, acceptance
   criteria, and risks. A WP should be executable without re-deriving the
   context — if you find the context stale, fix the plan document in the same
   stack as the work.
4. On completion: update the status cell, link the PR(s), move any shipped
   design content into `math-evolution.md` Part I, and file discovered
   follow-on work as a new WP row (not as a chat note, not — for now — as a
   Sortit issue).

## Mapping to the strategy doc

`math-evolution.md` Part II describes five tracks; this plan resequences them
into stages by dependency: **Track D** (hardening) splits across Stage 1 (core
guards), Stage 3 (eval expansion), and Stage 6 (long tail); **Track A**
(themes) is Stage 2; **Track B** (overlays) is Stage 4; **Track C** (map) is
Stage 5; **Track E** (cheap-model economics + learned tag layer) is Stage 7.

## The queue

Stages are strictly ordered by dependency, not importance. Within a stage,
work top-to-bottom unless a WP's dependency note says otherwise.

### Stage 1 — Harden the shipped core ([10-stage-1-hardening.md](./10-stage-1-hardening.md))

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-101 | Put the ridge default under the golden baseline | S–M | — | merged (PR #201) |
| WP-102 | Stale-text and misleading-comment sweep | XS | — | merged (PR #202) |
| WP-103 | Revision-keyed ridge decomposition cache + solver concurrency | M–L | WP-101 | merged (PR #204) |
| WP-104 | Standardize per-tag drift deltas | S | — | merged (PR #203) |
| WP-105 | GCV solver cost and factorization consistency | S–M | WP-101 | merged (PR #205) |

### Stage 2 — Themes to production ([20-stage-2-themes.md](./20-stage-2-themes.md))

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-201 | Corpus loadings source for themes | S | WP-103 | merged (PR #206) |
| WP-202 | Revision-keyed theme cache | S | WP-201 | merged (PR #206) |
| WP-203 | Theme identity stability across refreshes | M | WP-202 | merged (PR #212) |
| WP-204 | Themes debug API | S | WP-203 | merged (PR #212) |
| WP-205 | Theme labeling | S–M | WP-204 | merged (PR #212) |
| WP-206 | NMF convergence, K, and quality telemetry | S–M | WP-202 | merged (PR #212) |
| WP-207 | Export full H rows from issuethemes | S | — | todo |
| WP-208 | Dev-corpus soak and qualitative read | S + calendar time | WP-204, seeded dev server | todo |

WP-207/208 are follow-ons discovered while shipping Stage 2 (flagged in the
WP-203/204 outcomes). WP-207 should land before WP-402/403 build on top-5-tag
approximations of H; WP-208 gates promotion of any Stage 4 surface out of
debug tier and gates WP-501.

### Stage 3 — Evaluation expansion ([30-stage-3-eval.md](./30-stage-3-eval.md))

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-301 | Real-embedding fixture | M | — | shipped (in stack) |
| WP-302 | Explore and person-recommendation eval coverage | M | — | shipped (in stack) |
| WP-303 | Map-projection quality metric | M | — | shipped (in stack) — neighborhood/silhouette/stability baselined; real-embedding overlap 0.33–0.40 is the layout ceiling Stage 5 must beat |
| WP-304 | Ridge default re-examination on real geometry | M | WP-301 | shipped (in stack) — verdict: ranking default switched to **uncentered** tag-space ridge |

Stage 3 can interleave with Stage 2 — it shares no code with it. WP-303 gates
Stage 5; WP-301 gates believing any new fixture-derived number.

### Stage 4 — Planning overlays ([40-stage-4-overlays.md](./40-stage-4-overlays.md))

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-401 | Person profile on validated loadings (shadow) | M | WP-204 | todo |
| WP-402 | Issue-set theme coverage (iteration lens) | S–M | WP-204 | todo |
| WP-403 | Gap analysis | S | WP-402 | todo |
| WP-404 | Person-to-theme fit | S | WP-401, WP-402 | todo |
| WP-405 | Theme drift over time + snapshot decision | M–L | WP-402 | todo |
| WP-406 | Theme identity + label persistence | S–M | WP-204 | todo |

Stage 4 WPs run at debug tier on Stage 2's fixture-soak evidence alone;
**promoting any surface out of debug tier requires WP-406 (identities and
labels must survive restarts) and WP-208 (dev-corpus soak held)**.

### Stage 5 — Map projection on themes ([50-stage-5-map.md](./50-stage-5-map.md))

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-501 | PCA-on-W projection, dark, behind debug flag | M | WP-208, WP-303 | todo |
| WP-502 | Map flip: soak, compare, switch default | S | WP-501 | todo |

### Stage 6 — Long horizon ([60-stage-6-long-horizon.md](./60-stage-6-long-horizon.md))

Not sequenced against each other; schedule opportunistically once Stages 1–2
are done. Ordered roughly by value.

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-601 | Per-issue tag dismiss affordance → `dismiss` negations | M | — | todo |
| WP-602 | Co-occurrence negative evidence: wire or retire | S–M | WP-601 | todo |
| WP-603 | Negative-evidence magnitude calibration | M | WP-601, data | todo |
| WP-604 | Covariance shrinkage: Ledoit–Wolf or documented heuristic | S–M | WP-303 | todo |
| WP-605 | Authority/hubness slopes + adaptive specificity k | S | WP-301 | todo |
| WP-606 | GCV in the K ≥ D regime | M | — | todo |
| WP-607 | Outcome supervision spike | L | Stage 4 | todo |
| WP-608 | Curation & memory math whitepaper | M | — | todo |

### Stage 7 — Cheap-model economics and the learned tag layer ([70-stage-7-cheap-models-learned-layer.md](./70-stage-7-cheap-models-learned-layer.md))

Added 2026-08-07 (claims verified against code that date). Strategy: shift the
heavy lifting from expensive LLM opinion onto cheap models plus the math layer
— see math-evolution.md Track E. Interleaves freely with Stages 4–6 (no shared
code except WP-704's touch on the ranking caches).

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-701 | Embedding call hygiene: one embed per analysis | S | — | todo |
| WP-702 | Tagging-fidelity eval (the analyzer instrument) | M | — | todo |
| WP-703 | Cheap-first analyzer tiering with escalation | M | WP-702 | todo |
| WP-704 | Learned tag matrix `T` (anchored transposed ridge) | M–L | WP-301 (shipped) | todo |
| WP-705 | Mixed-kind fixture (documents / ideas / tasks) | M | WP-704 first study | todo |
| WP-706 | Embedding-model qualification harness + migration checklist | S–M | WP-606 for small-D | todo |

## Dependency graph

```
WP-102 (stale text) ─── standalone, anytime
WP-104 (drift deltas) ── standalone, anytime

WP-101 (ridge baseline) ──► WP-103 (decomp cache) ──► WP-201 ─► WP-202 ─► WP-203 ─► WP-204 ─► WP-205
                       └──► WP-105 (GCV cost)                      └────────► WP-206      │
                                                                                          ▼
WP-301 (real fixture) ─► WP-302 (explore/person eval) ─► Stage 4: WP-401..WP-405 (debug tier)
WP-207 (full H export) ──────────────────────────────────┘  (land before WP-402/403)
WP-406 (identity+label persistence) ─┬─► promotion of any Stage 4 surface out of debug
WP-208 (dev-corpus soak) ────────────┴─► WP-501 ─► WP-502  ◄── WP-303 (map metric)

WP-701 (call hygiene) ── standalone, anytime
WP-702 (tagging eval) ─► WP-703 (cheap-first tiering)
WP-301 (shipped) ──────► WP-704 (learned T) ─► WP-705 (mixed-kind fixture)
WP-606 (K ≥ D GCV) ────► WP-706 (embedding-model qualification)
```

**Current position (2026-08-07):** Stages 1–3 shipped (WP-101..WP-304).
Stage 7 added — the cheap-model + learned-layer program. Recommended path from
here: **WP-701** (standalone cost win, pays for everything after it), then
**WP-702 → WP-703** (instrument, then the cheap-first flip) with **WP-704**
running in parallel (independent lane; the learned basis is also what makes a
cheaper analyzer safe — the geometry corrects noisy anchors). WP-705 follows
WP-704's first study. Stage 4 overlay work remains open and interleaves
freely; WP-208's seeded dev server still accrues calendar time and should
start whenever one exists. WP-606 moves up in priority if WP-706 ever chases a
small-dimension embedding model.

## Why this order (one paragraph per stage)

**Stage 1** (done) retired the risk we were carrying: the production ranker
(ridge) had no regression guard — the committed golden baseline exercised the
rank-1 path only. Nothing else in this program was safe to build until the
thing it builds on was guarded. The decomposition cache (WP-103) is both a
perf fix and the load-bearing dependency for themes.

**Stage 2** (done) turned the built-but-unwired `internal/issuethemes` factorizer into
a consumable corpus artifact. Theme *identity stability* (WP-203) is the
critical item — without it every overlay in Stage 4 is built on themes that
silently reshuffle between refreshes.

**Stage 3** buys trust: the current fixture generates embeddings from tag sums
and structurally favors the tag-space model, the harness has zero coverage for
explore/person/map, and Stage 5 cannot even measure the regression it risks.

**Stage 4** is the product payoff — the planning overlays the whole program
exists for. It consumes stable themes (Stage 2) and honest evals (Stage 3).

**Stage 5** is the single riskiest user-facing change (the map is muscle
memory), so it goes last, gated on theme stability soak and a map metric.

**Stage 6** is the long tail: new negative-evidence sources, calibration,
inherited weak spots, outcome supervision, and the undocumented curation math.
It keeps the program honest for as long as anyone wants to keep working it.

**Stage 7** attacks cost and the R² gap as one program: cheap models do the
first pass because the math layer (verifier gate, drift, ridge refinement, and
— new — a per-corpus *learned* tag basis) can correct loose anchors, and the
learned basis is measured under the same pre-committed-rule discipline that
re-decided the ridge default. It also carries the generalization instrument
(mixed-kind fixture) for the documents/ideas/tasks direction.

## Status legend

`todo` → `in progress (branch)` → `shipped (PR #)` → `folded into Part I`
(when `math-evolution.md` Part I absorbs the as-built description).
