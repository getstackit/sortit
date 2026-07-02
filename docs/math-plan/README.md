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

`math-evolution.md` Part II describes four tracks; this plan resequences them
into stages by dependency: **Track D** (hardening) splits across Stage 1 (core
guards), Stage 3 (eval expansion), and Stage 6 (long tail); **Track A**
(themes) is Stage 2; **Track B** (overlays) is Stage 4; **Track C** (map) is
Stage 5.

## The queue

Stages are strictly ordered by dependency, not importance. Within a stage,
work top-to-bottom unless a WP's dependency note says otherwise.

### Stage 1 — Harden the shipped core ([10-stage-1-hardening.md](./10-stage-1-hardening.md))

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-101 | Put the ridge default under the golden baseline | S–M | — | shipped (in stack) |
| WP-102 | Stale-text and misleading-comment sweep | XS | — | shipped (in stack) |
| WP-103 | Revision-keyed ridge decomposition cache + solver concurrency | M–L | WP-101 | shipped (in stack) |
| WP-104 | Standardize per-tag drift deltas | S | — | shipped (in stack) |
| WP-105 | GCV solver cost and factorization consistency | S–M | WP-101 | shipped (in stack) |

### Stage 2 — Themes to production ([20-stage-2-themes.md](./20-stage-2-themes.md))

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-201 | Corpus loadings source for themes | S | WP-103 | todo |
| WP-202 | Revision-keyed theme cache | S | WP-201 | todo |
| WP-203 | Theme identity stability across refreshes | M | WP-202 | todo |
| WP-204 | Themes debug API | S | WP-203 | todo |
| WP-205 | Theme labeling | S–M | WP-204 | todo |
| WP-206 | NMF convergence, K, and quality telemetry | S–M | WP-202 | todo |

### Stage 3 — Evaluation expansion ([30-stage-3-eval.md](./30-stage-3-eval.md))

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-301 | Real-embedding fixture | M | — | todo |
| WP-302 | Explore and person-recommendation eval coverage | M | — | todo |
| WP-303 | Map-projection quality metric | M | — | todo |

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

### Stage 5 — Map projection on themes ([50-stage-5-map.md](./50-stage-5-map.md))

| WP | Title | Size | Depends on | Status |
|---|---|---|---|---|
| WP-501 | PCA-on-W projection, dark, behind debug flag | M | WP-203 soak, WP-303 | todo |
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

## Dependency graph

```
WP-102 (stale text) ─── standalone, anytime
WP-104 (drift deltas) ── standalone, anytime

WP-101 (ridge baseline) ──► WP-103 (decomp cache) ──► WP-201 ─► WP-202 ─► WP-203 ─► WP-204 ─► WP-205
                       └──► WP-105 (GCV cost)                      └────────► WP-206      │
                                                                                          ▼
WP-301 (real fixture) ── parallel ─────────────────────────────►  Stage 4: WP-401..WP-405
WP-302 (explore/person eval) ── parallel                                       │
WP-303 (map metric) ───────────────────────────► WP-501 ─► WP-502  ◄── WP-203 soak
```

## Why this order (one paragraph per stage)

**Stage 1** retires the risk we are already carrying: the production ranker
(ridge) has no regression guard — the committed golden baseline exercises the
rank-1 path only. Nothing else in this program is safe to build until the
thing it builds on is guarded. The decomposition cache (WP-103) is both a perf
fix and the load-bearing dependency for themes.

**Stage 2** turns the built-but-unwired `internal/issuethemes` factorizer into
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

## Status legend

`todo` → `in progress (branch)` → `shipped (PR #)` → `folded into Part I`
(when `math-evolution.md` Part I absorbs the as-built description).
