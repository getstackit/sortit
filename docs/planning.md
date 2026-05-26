# Quantitative Project Management Layer

This document sketches, from first principles, what a "quantitative project management" layer on top of Sortit's existing data substrate could look like. It is deliberately abstract — no math, no implementation details — and is meant to anchor longer-term design decisions about which capabilities to build, in what order, and why.

For the underlying scoring, search, and map mechanics, see [scoring-search-map.md](./scoring-search-map.md).

## The substrate

Per issue, Sortit has three things:

1. **Tag loadings.** Sparse, interpretable, signed, with provenance and evidence. This is *what* the work is about, in a basis humans can read.
2. **Embedding.** Dense, opaque, continuous. This is *how the work feels* relative to other work, including everything tags didn't capture.
3. **Lifecycle trajectory.** Status, posts, snapshots, links, operations. This is *what happened* to the work over time.

RAG and embeddings give you a single verb on this substrate: **find similar**. A quantitative PM layer needs a much wider verb set, and the tag basis is what unlocks it, because tags are the only axis that is simultaneously *interpretable*, *composable*, and *stable across time*. That is the core leverage; everything else follows from it.

## Capability tiers

The layer is best organized as four tiers, each building on the last.

### Tier 1 — Cartography

Treat the corpus as a map. Tags partition it into named regions; embeddings give the within- and between-region geometry. Region-level quantities become first-class:

- Mass: how many issues live here.
- Density: how packed they are in embedding space.
- Growth rate: net new issues over time.
- Closure rate: how quickly the region drains.
- Churn: refinement, splits, merges per issue.
- Age distribution: how long open issues have been open.
- Dead zones: regions where existing work doesn't fit any tag.

This is the dashboard layer — *where is the project right now*.

### Tier 2 — Per-issue X-ray

For one issue, derive a small handful of interpretable scalars from the triple:

- **Specificity** — how concrete this issue is versus its region's average.
- **Coherence** — whether its tags normally co-occur.
- **Definition** — whether its embedding sits cleanly in a region or drifts between several.
- **Connectedness** — links, parents, derived-from chains.
- **Lifecycle position** — where on its own arc it sits.

The output is a per-issue scorecard with named axes, each one falsifiable against the source text. This is what end users feel day-to-day.

### Tier 3 — Trajectory and drift

Tags + lifecycle let you track motion that embeddings alone can't name:

- Did this issue's tag profile shift after refinement?
- Is this region accelerating or decaying?
- Is a child issue actually still in its parent's region or has it walked away?
- Are co-occurring tags pulling apart or pulling together over time?

This is where "quantitative" stops being a snapshot and starts being a time series. It is also where contradiction signals — the negation work — earn their keep, because the most interesting motion is when a region starts shedding co-occurring tags or growing internal contradictions.

### Tier 4 — Outcome-conditioned reasoning

Tags + lifecycle outcomes (closed, duplicated, abandoned, split, merged, time-to-close) make tag and embedding features *predictive*, not just descriptive. The interesting verbs here aren't "find similar" but:

- Find similar **that closed in under a week**.
- What tag signatures predict duplicate-of?
- Which combinations of loadings precede a split?
- What regions historically generate parent-of links?

This is the layer that makes the system advisory rather than analytical, and the one with the most long-term defensibility.

## Governing principles

- **Tags are the basis, embeddings are the residual.** Anything you can explain in tag space, explain there — it's the part the user can argue with. Use embeddings to capture *what tags miss*, not as a substitute for naming things.
- **Every quantitative claim should have provenance back to evidence.** Tag relevance already follows this. The same discipline should apply to "this region is growing" (which issues, when?) and "this issue looks like a duplicate" (of what, on what features?).
- **Region > issue > pair.** The most novel unit of analysis is the *region* (a tag, a cluster, a slice). Issue-level is what users see; pair-level (similarity) is what RAG already does. The middle layer is the one that's underexplored and the one tags uniquely enable.
- **Lifecycle is the source of truth for "did it matter."** Without outcome data, all of this is descriptive geometry. Negation was step one of taking lifecycle signals seriously as feedback into the tag and embedding layers; later regression and factor work plug in there.

## Tier 1 — Where we are vs. where we want to be

This section grounds the Tier 1 vision in the actual codebase. It is a snapshot, so it will go stale; the intent is to capture the *shape* of the gap rather than a punch list.

### What we already have that's reusable

- **A map projection.** `internal/map` builds a tag-relevance matrix, applies tag-covariance smearing, weights by content confidence and maturity, runs PCA, and renders 2D issue points. This is the closest thing to a corpus view that exists.
- **Spatial clusters on top of the projection.** `internal/map/cluster.go` runs k-means on the projected points and produces a center, radius, and dominant tag per cluster. This is the only existing concept of "region" in the system.
- **Per-issue analytics primitives.** `internal/issueanalytics` exposes freshness (90-day half-life), velocity (30-day window), maturity, authority, hubness, and content confidence — all computed per issue.
- **A closure timeline.** `closed-factor-attribution-chart` buckets closed issues by tag relevance over time, in fixed windows (1w/1m/3m/6m/1y/all). It is the only time-series surface today.
- **A tags page.** `src/app/tags` shows tag embeddings, specificity, and merge candidates. It is structure-only — no issue counts, no growth, no closure.

### The shape gap

Almost everything we have is **per-issue**. Tier 1 needs **per-region**. The five region-level quantities listed in the Tier 1 description — mass, density, growth, closure, churn, age, dead zones — have no implementation. Region itself is not a first-class data shape: today a "region" is either an implicit dominant-tag annotation on a spatial cluster, or it's nowhere at all.

The two most important missing concepts:

1. **A `Region` abstraction.** Today "tag" and "spatial cluster" both informally play this role, but neither has a metric surface attached. We need a unit of analysis that can hold mass, density, growth, closure, churn, age, and orphan-rate — addressable by either a tag (interpretable) or a cluster (geometric), and stable enough to be sliced over time.
2. **Region-scoped time series.** The lifecycle metrics on issues are *aggregate counts* (refinement count, transition count), not a *history*. We can answer "how many transitions has this issue had" but not "how many issues entered or left this region per week." The append-only event log has the raw material; nothing aggregates it.

### What's missing entirely

- **Mass, growth, closure per tag/cluster.** No query, no projection, no UI surface.
- **Density in embedding space per region.** The projection knows where points sit but never computes within-region spread, inter-region separation, or compactness.
- **Churn at region scope.** Per-issue refinement counts exist; per-region churn rates do not.
- **Age distribution and aging curves.** No bucketing of open issues by creation date or time-in-region.
- **Dead zones.** The verifier surfaces tag-issue misalignment at enrichment time, but there is no orphan signal at the corpus level — no view that says "these N issues are not well-covered by any tag in the catalog."
- **Per-tag dashboards.** The tags page shows semantic geometry only.

### What exists but is the wrong shape

- **Per-issue scoring blends absorb signals that should also be region-level metrics.** Velocity, authority, and hubness are computed per issue and consumed inside search ranking; none of them roll up to region scope. Authority in particular is only ever a search booster — it should also be a per-region property (which tags or clusters concentrate the canonical landing points).
- **Spatial clusters compete with tag regions instead of complementing them.** K-means on the PCA projection produces "regions" with no naming guarantee and no relation to the tag taxonomy. A reader looking at a cluster gets a label that is the cluster's dominant tag, but the cluster itself is not addressable as "the auth region" — it is a geometric blob that happens to be auth-heavy this week. Tier 1 wants both: tag regions as the interpretable primary axis, with spatial structure as the geometric refinement *inside* a tag region.
- **The closure timeline is the right idea at the wrong scope.** It only covers closures and only at the corpus level with tag attribution. The same shape generalized — events × region × time — would underpin growth, churn, and age curves.
- **Map projection weights mix measurement with rendering.** Content confidence and maturity are baked into the PCA weighting so the projection looks better. That's fine for the map, but it means the same weights are not available as independent region-level metrics; they live inside a rendering function.

### What to remove or revisit

- **`IssueLifecycleMetrics.Stability`** is computed inside maturity but never read by any consumer. Either define it sharply and surface it, or delete it.
- **Authority-only-in-search.** Either make authority a first-class region property or stop computing it on issues that never participate in search ranking.
- **Hubness as a cluster-label tiebreaker** (`internal/map/cluster.go`) is opaque — it weights candidate tags for cluster naming without a stated rationale. Revisit once `Region` is a first-class concept; the right tiebreaker for cluster naming may just be regional mass.
- **The k-means-on-PCA cluster surface** is worth revisiting once tag regions exist. It may be redundant, or it may shift to a "show me the geometric sub-structure inside this tag region" role rather than a top-level segmentation.
- **The scoring primitive zoo** (`Freshness`, `Velocity`, `Maturity`, `Authority`, `Hubness`, `ContentConfidence`) should be audited as a set. Each was added for a specific consumer; some are general enough to roll up to regions, others are search-specific. Without this audit, Tier 1 will accidentally re-implement the same signals at a different scope.

### A staged path to Tier 1

1. **Introduce `Region` as a first-class shape**, initially as "a tag" (the interpretable case). Region for a cluster comes later, once tag regions are working.
2. **Compute mass and age first.** These need only the existing issue store; they are the cheapest wins and produce immediately useful per-tag dashboards.
3. **Add growth and closure** by aggregating the existing event log per region per time bucket. Generalize the closure timeline into a single `events × region × window` query.
4. **Add density and dead zones** by reusing the existing embedding store. Density is intra-region embedding spread; dead zones are issues whose nearest tag in embedding space is below a threshold.
5. **Add churn** from refinement/split/merge events at region scope.
6. **Promote authority and hubness to per-region rollups** alongside the per-issue values, so search and cartography draw from the same primitives instead of forking them.
7. **Revisit spatial clusters** once the above is in place. They likely become "sub-region structure within a tag region" rather than a top-level segmentation.

## Picking where to start

- **Tier 1** is the easiest to demo and the most legible to a new user.
- **Tier 2** is where day-to-day users feel the system most.
- **Tier 3** is where the math evolution work (anchored ridge regression, co-occurrence anti-correlation, signed loadings) starts to compound.
- **Tier 4** is where long-term defensibility lives, and the only tier that genuinely requires lifecycle outcome data to be reliable.

The right first product surface is a Tier 1 view backed by Tier 2 scorecards, with Tier 3 and Tier 4 wired up underneath as the corpus and outcome history grow.
