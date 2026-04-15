# Tagging Accuracy Plan

## Goal

Improve Sortit's tag assignment so tags are more accurate, more specific, and
more reusable across issues, without regressing the current "dump text in and
the system figures out the rest" workflow.

This document is an implementation plan. It turns the current tagging proposal
into a sequence of concrete changes that can be shipped incrementally.

## Problems To Solve

The current tagging pipeline has several structural limits:

- Tagging is mostly a single-pass LLM classification over the current catalog.
- The starter taxonomy is broad, so the model often lands on broad tags even
  when a more precise concept exists or should exist.
- Suggested tags are promoted into the live catalog after a single analysis,
  which can admit noisy one-off tags.
- Re-enrichment hint tags are chosen from the issue's previously stored
  embedding, which can become stale after canonicalization/refinement.
- The LLM sees a long taxonomy with limited structural signal: tag definitions,
  specificity ordering, and hint tags exist, but there is no explicit
  hierarchy and no examples of what good tagging looks like.
- Specificity is used mainly as a post-processing adjustment rather than as a
  first-class part of candidate selection and verification.
- Some frontend surfaces still rely on legacy hardcoded "generic tag" logic
  instead of the persisted specificity model.

## Desired Outcome

The tagging system should behave like this:

1. Start from a strong reusable catalog, not just broad defaults.
2. Narrow the model's attention to the most plausible candidate tags for the
   current issue.
3. Verify assigned tags against the issue text and remove weak matches.
4. Treat new tags as deliberate taxonomy growth, not a byproduct of one issue.
5. Use one specificity model consistently across backend ranking, display, and
   UI guidance.
6. Produce measurable evidence that tagging quality is improving.

## Non-Goals

- Replacing the entire enrichment system in one rewrite
- Eliminating broad tags entirely
- Preventing all new tag creation
- Designing a perfect ontology before shipping improvements
- Making tagging depend on manual triage for the happy path

## Principles

### Reuse before invent

The system should prefer the best existing reusable tags before proposing new
ones.

### Specificity without overfitting

Tags should be narrow enough to distinguish issues, but not so narrow that they
only ever apply once.

### Taxonomy growth is a product decision

Adding a new tag changes the structure of the workspace. That decision should
require more evidence than a single model response.

### Retrieval before generation

When the catalog is non-trivial, the model should not reason over the full tag
set blindly if a smaller candidate set can be retrieved first.

### Instrument everything

The system already has useful residual and low-R² diagnostics. Improvements
should be evaluated with explicit before/after signals rather than gut feel.

### Prefer reversible rollout

Changes to retrieval, prompting, verification, and taxonomy growth should be
shippable behind flags or side-by-side evaluation paths until the benchmark
shows clear improvement.

## Current Pipeline Summary

Today the pipeline is roughly:

1. Build a taxonomy from stored tags or broad defaults.
   - On create, if the user supplied explicit tags, the current code can reduce
     the taxonomy to those tags only.
2. Annotate a few tags as embedding-based hints (similarity to issue embedding).
3. Ask the LLM to score tags in one pass.
4. Persist any returned suggested tags into the catalog.
5. Attenuate low-specificity tags (relevance × 0.6 when specifics are present).
6. Select up to 3 display tags ranked by relevance × 0.5 + specificity × 0.5.

This is workable, but it makes accuracy and specificity too dependent on a
single model pass.

Implementation status uses checkboxes below:

- `[x]` verified complete in the current codebase / CLI
- `[ ]` not yet complete or not yet verified end-to-end

### Recent changes (shipped)

- [x] Re-enrichment now uses the full catalog taxonomy instead of restricting
  to the issue's current tags.
- [x] `POST /issues/{id}/re-enrich` and `sortit issues re-enrich` allow
  triggering re-classification without a fake refinement post.
- [x] Embedding-based hint tags are annotated on the taxonomy during
  enrichment.
- [x] R² diagnostics are exposed on the issue detail page and standard API
  routes.
- [x] The tagging prompt was updated to surface high-affinity hint tags and
  encourage more specific assignments.
- [x] Ledoit-Wolf shrinkage applied to the tag covariance matrix to regularize
  factor decomposition as the catalog grows.
- [x] Map availability status and minimum issue count thresholds added for map
  rendering.
- [x] Memory layout and hash lookup overhead optimized in the factor model.

### Known remaining issues

- Issue tag score persistence now preserves `suggested` / `description`
  provenance plus verifier metadata (`candidateSources`, `alignment`,
  `specificity`, `verificationVerdict`, `verificationReason`,
  `dominatedBy`, `dominanceGap`), but some SQL/debug/query surfaces still only
  reason about a narrower subset of that persisted shape.
- Some SQL/debug/query surfaces still reason about issue tag scores primarily
  as `{tag, relevance}` records. That is sufficient for current filtering and
  ranking, but later verification and review features will require explicit
  query-level support for the wider persisted shape.
- Generic attenuation is unconditional: any tag with specificity < 0.3 gets a
  flat 0.6× penalty whenever a specific tag is present, regardless of context.
- Display is capped at 3 tags with a hard 0.2 relevance floor. Tags between
  0.08 (AI floor) and 0.2 are persisted but invisible.

## Cross-Cutting Requirements

These requirements apply across all phases.

### Data model changes must be explicit

The plan introduces new persisted concepts and should name them directly rather
than leaving them implicit in service code.

Expected additions:

- tag provenance on issue assignments (required by Phase 1):
  - whether a tag came from the catalog or was model-suggested
  - the model-provided description for suggested tags
- verification metadata on issue tag assignments (required by Phase 3):
  - embedding alignment score
  - verification verdict (keep, down-rank, follow-up, flagged)
  - whether the tag came from retrieval, anchor set, or explicit user input
  - this metadata must be persisted so Phase 6 review queues can surface
    flagged tags and so verifier effectiveness can be measured over time
- tag lifecycle state in the catalog (required by Phase 4):
  - `active`
  - `proposed`
  - `merged`
  - `dismissed` or equivalent
- optional evaluation/debug artifacts:
  - benchmark fixtures
  - exemplar issue IDs for few-shot prompting

These schema changes are prerequisites for their respective phases. Phase 1
cannot preserve provenance without a place to store it. Phase 3 cannot flag
tags for review without persisted verdicts. Phase 4 cannot gate promotion
without lifecycle states. Plan migration work to land before or alongside the
phase that requires it.

APIs and UI surfaces should define whether they include only `active` tags or
also surface `proposed` tags in review/debug views.

The plan should also call out the implementation blast radius for these
concepts:

- issue tag score JSON shape in append-only and Postgres persistence
- SQL readers/writers and search queries that deserialize `tag_scores_json`
- API response types consumed by the frontend
- any derived search/index fields that flatten tags into searchable terms

### Prompt token budget

The plan progressively adds content to the tagging prompt: few-shot examples
(Phase 1.5), retrieval shortlists with descriptions (Phase 2), and optional
verification re-checks (Phase 3). The current prompt (`buildOpenAITaggingPrompt`
in `openai.go`) has no token budget awareness. With `gpt-5-mini` as the default
model, cumulative prompt growth across phases could approach context limits for
issues with long canonical text.

Mitigations:

- Track prompt token count as a diagnostic metric from Phase 0 onward.
- Phase 1.5 should cap few-shot example text length (e.g., truncate canonical
  text to first 200 tokens per example).
- Phase 2's shortlist naturally reduces taxonomy tokens vs the full catalog,
  partially offsetting few-shot growth.
- Set an explicit token budget ceiling for the tagging prompt and fail loudly
  (or fall back to a minimal prompt) if exceeded.

### Rollout safety

The following capabilities should be independently switchable:

- few-shot prompting
- retrieval-first shortlist construction
- embedding-based verification
- proposed-tag gating

Each capability should support:

- benchmark comparison against the current baseline
- targeted local testing on selected issues
- safe disablement if precision or recall regresses

## Plan

The phases below are ordered to improve quality early while keeping taxonomy
changes reversible until evaluation is in place.

## Phase 0: Measure The Current Failure Modes

Before changing the classifier, make quality visible.

### Deliverables

- [x] A small labeled benchmark set of real issues with expected tags, drawn
  from the current corpus. Select issues spanning different failure modes:
  - [x] low-R² issues (tags explain little)
  - [x] high-relevance / low-alignment tags (AI confident but wrong direction)
  - [x] residual-near unassigned tags (good tag exists but was missed)
  - [ ] well-tagged high-R² issues (positive examples)
- [x] A repeatable evaluation command (`sortit debug eval-tags` or test fixture)
  that runs the tagger against the benchmark and reports precision, recall, and
  per-issue diffs
- [ ] A review queue surfacing actionable issues from existing diagnostics,
  with `sortit issues re-enrich` as the action mechanism
- [x] Reuse the existing R²/debug infrastructure as the starting point rather
  than rebuilding it from scratch:
  - [x] low-R² issue listing
  - [x] high-relevance / low-alignment diagnosis
  - [x] nearest residual-tag suggestions
  - [x] residual-neighbor clustering

### Why first

Without a benchmark, the system will drift between "more specific" and "more
random" without a reliable way to tell the difference.

### Success criteria

- We can compare two tagger versions on the same issue set and see which is
  better
- We can list the top classes of failures instead of relying on anecdotes
- We can measure tagger stability: running the same issue through the pipeline
  twice produces the same tags (catches regressions from model updates,
  embedding drift, or prompt changes that precision/recall alone would miss)

## Phase 1: Fix The Existing Pipeline Before Adding More Complexity

Make the current architecture less error-prone.

### Deliverables

- [x] Recompute hint tags from a fresh embedding of the canonical text during
  enrichment, not from the previously stored `issue.Embedding`. Concretely:
  embed the canonical text first, then call `AnnotateHints` with that fresh
  embedding before passing the taxonomy to the LLM.
  - Implementation note: `AnalyzeIssueData` currently bundles embedding and
    tagging into one call. Fixing stale hints requires a separate embed-only
    call *before* the tagging call, since the fresh embedding must be available
    for hint selection before the tagger runs. This adds one API call to the
    enrichment path.
- [x] Resolve the create-path taxonomy asymmetry explicitly:
  - [ ] either keep "explicit tags only" for create as an intentional product
    rule
  - [x] or change create to use the full catalog plus explicit tags as anchors
    so create, refine, combine, and re-enrich are comparable
- [x] Preserve `Suggested` and `Description` metadata from AI tag scores
  through persistence so tag provenance is inspectable.
- [ ] Land the schema/query work needed for that provenance:
  - [x] extend persisted issue tag-score shape
  - [x] update append-only and Postgres readers/writers
  - [ ] update remaining SQL/query/API surfaces that still assume
    `{tag, relevance}` only
- [x] Add a server-side relevance floor (0.08) in `IssueTagScoresFromAnalysis`
  to match the prompt instruction, so sub-threshold tags are never persisted
  even if the model returns them.
- [x] Ensure create, refine, combine, and re-enrich flows all use the same
  specificity-aware ranking and display rules.
  - Concretely, remove the current create/refine use of legacy `displayTags`
    when specificity-aware display is intended.
- [ ] Remove remaining frontend reliance on hardcoded generic bucket tags and
  use persisted specificity consistently.

### Why this phase

These are low-risk corrections to the existing design. They improve precision
without changing the product model.

### Success criteria

- Hint tags are derived from current issue meaning, not stale state
- UI and backend agree on what counts as a broad vs specific tag
- Tag provenance (suggested vs taxonomy, original description) is preserved

## Phase 1.5: Add Few-Shot Examples To The Tagging Prompt

The single highest-leverage prompt improvement that does not require
architectural changes.

### Deliverables

- [x] During enrichment, select up to 3 semantically close curated exemplars
  for the current issue.
- [ ] Replace the curated exemplar pool with live high-R² corpus selection, or
  persist an explicit exemplar pool of known-good issue IDs refreshed from the
  corpus over time.
- [x] Include exemplar issue text and assigned tags as few-shot examples in
  the tagging prompt, before the current issue text.
- [x] Format examples to show what good specific tagging looks like:
  ```
  Example 1:
  Issue: "Backfill and dual-write embedding data so vector columns stay in sync"
  Tags: data-persistence (0.92), database-migration (0.88), integration (0.75)
  ```

### Current status

The codebase now ships a curated exemplar pool and prompt wiring for few-shot
examples, and current tuning work is improving the benchmark, but the release
gate is still open.

Latest local benchmark check on March 26, 2026 (`fixture=corpus`) after
exemplar-admission and prompt-tuning changes:

- retrieval-shortlist + verifier: precision 0.788, recall 0.929
- full-catalog + verifier: precision 0.743, recall 0.929
- retrieval-shortlist without verifier: precision 0.813, recall 0.929

That means Phase 1.5 is now showing a local precision win over the current
full-catalog baseline, but it still needs repeated runs and continued tuning
before the phase should be treated as complete.

### Prerequisites

Selecting few-shot examples requires querying issues by R² and embedding
similarity. Today R² is computed on-the-fly during factor decomposition but is
not persisted or indexed on the issue. This phase needs either:

- A persisted R² field on issues (updated after each enrichment or periodically)
  so high-R² candidates can be retrieved efficiently, or
- A precomputed exemplar pool maintained out-of-band (curated set of issue IDs
  with known-good tagging).

The exemplar pool approach is simpler to start with and avoids coupling few-shot
selection to a new index.

### Design rules

- Only use examples with R² ≥ 0.3 and no obvious verifier warnings:
  - no high-relevance / low-alignment tags
  - no unresolved proposed tags
  - no known merge-candidate ambiguity on the key tags
- Prefer examples that share at least one candidate tag with the current
  retrieval set, so the model sees the tag used in context.
- Cap at 3 examples to limit prompt size. Prioritize diversity of tag axes
  over quantity.
- Examples should be refreshed as R² scores change — they are not static.
- Prefer a curated exemplar pool over unrestricted corpus sampling when such a
  pool exists. The system should not teach itself from low-quality labels.

### Why this phase

The model currently receives taxonomy definitions but never sees what correct
tagging looks like in practice. Few-shot examples are the standard technique for
improving LLM classification consistency and are additive to every later phase.

### Success criteria

- Benchmark precision improves on the Phase 0 evaluation set
- Fewer issues tagged with only broad tags when specific examples were available

## Phase 2: Add Retrieval-First Candidate Selection

Stop asking the model to classify against the entire catalog equally.

### Deliverables

- [x] Embed the issue's canonical text and retrieve the top-k most similar
  catalog tags by cosine similarity.
- [x] Build a tagging candidate set from:
  - [x] top ~10–15 nearest catalog tags by embedding similarity
  - [x] a small set of broad anchor tags across key axes (issue kind, failure
    mode, platform) to preserve recall on major dimensions
  - [x] user-supplied explicit tags when present
- [x] Pass that shortlist to the LLM instead of the entire catalog.
- [x] Retain the full catalog as a fallback path for debugging and controlled
  A/B comparison via the benchmark.
- [x] Introduce an explicit candidate-set builder rather than treating
  retrieval as only "hint tags in the prompt." Candidate selection should
  produce structured provenance for each candidate source:
  - [x] retrieval
  - [x] anchor set
  - [x] explicit user input
  - [x] fallback/full-catalog path for debug

### Relationship to hint tags

Phase 2 subsumes the Phase 1 hint mechanism. When the taxonomy *is* the
retrieval shortlist, every tag was already selected by embedding similarity —
marking some of them as "hints" adds no incremental signal. The current
implementation keeps `AnnotateHints` as a lightweight annotation layer for the
debug/analyze surfaces, but it should still be reduced further over time rather
than treated as the primary selection mechanism.

### Design rules

- The shortlist should be large enough to preserve recall, but small enough to
  force sharper decisions. Start with 15 retrieved + 5 anchors and tune against
  benchmark results.
- Broad anchor tags should remain available so the model can still express
  issue kind and major dimensions.
- The retrieval step is local (cosine similarity over cached tag embeddings) and
  adds negligible latency.

### Why this phase

This is the highest-leverage change for specificity. It changes the model's
choice set instead of only post-processing its outputs. A model choosing among
15 relevant candidates makes sharper decisions than one scanning 40+ tags of
mixed relevance.

### Success criteria

- More issues receive specific existing tags instead of generic ones
- Fewer clearly relevant catalog tags are missed when they are semantically
  close to the issue
- Benchmark recall does not regress vs full-catalog baseline

## Phase 3: Add Embedding-Based Verification

Verify tag assignments using the signals we already compute, without depending
on a full second LLM pass.

### Deliverables

- [x] After the LLM scores tags, compute verification features for each
  assigned and nearby unassigned tag:
  - [x] tag embedding alignment with the issue embedding
  - [x] tag specificity
  - [x] whether the tag came from retrieval, anchor set, or explicit user input
  - [x] whether a nearby unassigned tag strongly dominates a weak assigned tag
  - [x] verbatim source-text evidence quotes the model attached to the tag,
    plus a count of how many of those quotes were confirmed to appear in the
    issue text after lightweight normalization (case, whitespace, smart
    punctuation)
- [ ] Use these features to produce a verification verdict:
  - [x] keep
  - [x] down-rank (now also fires when a confidently-assigned tag has no
    matched evidence quote, or when the model returned no quotes for a
    high-relevance tag)
  - [ ] ask targeted follow-up
  - [x] flag for debug/review (also fires when the model fabricates evidence
    quotes for a high-relevance tag, or when a *suggested* tag arrives without
    any supporting source-text evidence)
- [ ] After verification, check whether a high-alignment unassigned catalog tag
  exists (residual-near tag). If so, add it as a candidate and optionally
  re-score with a targeted LLM check for that single tag.
- [ ] Gate the optional LLM re-check behind a confidence threshold: only invoke
  it when the embedding alignment gap between the best unassigned tag and the
  worst assigned tag exceeds a margin.

### Design rules

- The primary verification pass must be deterministic and free — no API calls.
- Embedding alignment is a strong signal, but not ground truth. Broad tags such
  as issue kind, workflow, or platform may be correct even with modest
  alignment.
- Hard drops should be reserved for clearly dominated cases, for example:
  a low-specificity or surface-level assigned tag with weak alignment when a
  semantically nearby unassigned tag is much better aligned.
- For many cases, down-ranking or flagging is safer than deletion.
- The optional LLM re-check is a single focused question ("Does tag X apply to
  this issue?"), not a full re-classification. It runs only when the embedding
  signals a likely miss.
- Verification should prefer dropping a weak specific tag over keeping a wrong
  one.

### Schema decisions

The cross-cutting data model section lists verification metadata as a
requirement, but the storage location and lifecycle need to be decided before
implementation:

- **Location**: Do verdicts live on the issue-tag join (extending
  `TagRelevance`) or in a separate verification-results table? Extending the
  join is simpler but widens every tag score read/write path. A separate table
  keeps the hot path untouched but requires joins for review queries.
- **Mutability**: Can re-enrichment overwrite a previous verdict? If so, the
  old verdict should be logged for trend analysis (Phase 6). If not, verdicts
  accumulate and need a resolution policy.
- **Manual overrides**: If a user explicitly assigns or removes a tag, does that
  override the verifier verdict permanently, or can a future re-enrichment
  re-evaluate it?

### Why this phase

Single-pass classification is the biggest source of "plausible but wrong"
labels. Embedding alignment catches exactly this failure mode — the R²
diagnostics already proved it on `cleanup` (relevance 0.85, alignment 0.079).
Using an existing signal avoids the cost and latency of a full second LLM pass.

### Success criteria

- Fewer high-relevance tags with poor embedding alignment
- Fewer obviously wrong tags surviving into persisted issue state
- Benchmark shows improved precision without meaningful recall loss

## Phase 4: Change How New Tags Enter The Catalog

Treat taxonomy expansion as a controlled workflow.

### Deliverables

- [ ] Introduce a `proposed` or equivalent pre-canonical tag state.
- [ ] Do not auto-promote a newly suggested tag into the main catalog after a
  single issue. `EnsureStoredTags` should persist proposed tags separately from
  active catalog tags.
- [ ] Add promotion rules such as:
  - [ ] repeated appearance across multiple issues (≥ 3)
  - [ ] strong residual clustering around the same concept
  - [ ] explicit user acceptance via a review surface
- [ ] Add rejection/merge paths for noisy proposed tags.
- [ ] Proposed tags should still be assigned to the originating issue and
  scored, but not appear in the taxonomy for other issues until promoted.
- [ ] Define visibility and indexing rules explicitly:
  - [ ] whether proposed tags appear on issue detail pages
  - [ ] whether proposed tags participate in search
  - [ ] whether proposed tags are included in debug/R² review surfaces
  - [ ] whether proposed tags are excluded from future taxonomy construction by
    default

### Implementation note

`EnsureStoredTags` currently treats all tags uniformly — it upserts with
embeddings and descriptions, with no concept of lifecycle state. Introducing
`proposed` vs `active` requires one of:

- Adding a `State` field to the `Tag` struct and filtering in every store
  operation (`IssueTaxonomy`, `AnnotateHints`/retrieval, specificity scoring).
- A separate proposed-tags store, keeping the active catalog path untouched.

The choice affects how `IssueTaxonomy` constructs the taxonomy, how retrieval
(Phase 2) handles proposed tags in similarity search, and whether proposed tags
participate in specificity neighborhood calculations. Decide before
implementing.

### Why this phase

The catalog quality determines the future accuracy ceiling of the system.
Noisy growth makes the classifier less precise over time. Tags like
`suggested-responsive-svg` (created from a single issue, never reused) clutter
the taxonomy and dilute the model's attention.

### Success criteria

- Fewer one-off tags in the active catalog
- New active tags have clearer descriptions and stronger reuse evidence

## Phase 5: Data-Driven Taxonomy Growth

Make taxonomy expansion systematic using the signals the factor model already
produces.

### Deliverables

- [ ] Periodically run residual clustering over all issues (the
  `residualNeighbors` infrastructure already identifies clusters sharing
  unexplained concepts).
- [ ] Before proposing a new tag for a residual cluster, run a
  merge-before-create check:
  - [ ] is there an existing active tag that already fits the cluster?
  - [ ] is there a proposed tag that should be promoted instead?
  - [ ] are multiple tags in the cluster actually aliases or merge candidates?
- [ ] When a residual cluster exceeds a size threshold (e.g., ≥ 4 issues with
  pairwise residual similarity > 0.4), automatically propose a tag:
  - [ ] Generate a candidate tag name and description from the cluster's common
    text patterns.
  - [ ] Surface the proposal in the review queue (Phase 6) with the cluster
    members as evidence.
- [ ] Periodically refine existing active tag descriptions using evidence from
  the corpus, not just manual intuition:
  - [ ] gather issues with high loading, high alignment, or repeated confident
    use of the tag
  - [ ] extract common positive patterns and recurring false-positive neighbors
  - [ ] update the tag description so it states what the tag captures and what
    it excludes, with examples grounded in real issue language
  - [ ] re-embed the tag after materially changing its description and
    re-evaluate retrieval and benchmark impact
- [ ] Periodically run a catalog-wide taxonomy maintenance pass over active tags
  using corpus evidence and LLM assistance:
  - [ ] collect, for each active tag:
    - [ ] representative high-confidence / high-alignment issues
    - [ ] recurring low-alignment false-positive issues
    - [ ] nearest semantically similar tags and likely merge candidates
    - [ ] usage frequency and common co-occurrence patterns
  - [ ] ask for an explicit maintenance recommendation per tag:
    - [ ] keep as-is
    - [ ] improve description
    - [ ] split into narrower reusable concepts
    - [ ] merge with another tag
    - [ ] demote or deprecate as a low-information tag
  - [ ] treat accepted changes as explicit taxonomy edits:
    - [ ] update descriptions in place when wording is the main problem
    - [ ] re-embed changed tags after material description edits
    - [ ] move deprecated or merged tags out of the active runtime taxonomy
    - [ ] re-evaluate retrieval quality and benchmark impact after each batch
- [ ] Grow the tag set along explicit reusable axes such as:
  - [ ] issue kind (bug, improvement, cleanup, investigation)
  - [ ] failure mode (crash, data loss, performance regression)
  - [ ] platform (iOS, web, API)
  - [ ] workflow (onboarding, export, search)
  - [ ] surface (dashboard, settings, editor)
  - [ ] subsystem or artifact (database, auth, enrichment pipeline)
- [ ] Improve tag descriptions so each tag states what it captures and
  excludes.
- [ ] Avoid exploding into cross-product tags unless the concept is truly
  stable and reusable.

### Why this phase

The residual clustering data already shows where the taxonomy has gaps. Using
it for tag proposals turns taxonomy expansion from an editorial guess into a
data-driven workflow with built-in evidence.

### Success criteria

- Fewer issues fall back to broad tags only
- Residual clusters increasingly point to concrete missing concepts instead of
  generic confusion
- New tags have supporting evidence from multiple issues

## Phase 6: Close The Loop With Debugging And Review

Use the existing diagnostics as an operating system for taxonomy quality.

### Deliverables

- [ ] A periodic review flow for:
  - [ ] lowest-R² issues, with `sortit issues re-enrich` as the action
  - [ ] recurring residual clusters (Phase 5 proposals)
  - [ ] proposed tags awaiting promotion or merge (Phase 4)
  - [ ] high-relevance / low-alignment tags flagged by verification (Phase 3)
- [ ] A dashboard or report showing:
  - [ ] share of issues tagged only with broad tags (specificity < 0.3)
  - [ ] share of issues with at least one specific tag (specificity ≥ 0.3)
  - [ ] tag reuse distribution (how many issues per tag)
  - [ ] aggregate R² trend over time
  - [ ] benchmark precision/recall trends across releases
- [ ] Targeted automatic re-enrichment when the taxonomy changes in a way that
  may affect tagging, for example:
  - [ ] a proposed tag is promoted to active
  - [ ] a tag description changes materially
  - [ ] retrieval configuration changes
  - [ ] a merge changes canonical tag identity
  - [ ] a residual cluster proposal becomes available for a known slice of
    issues
- [ ] Re-enrichment should prefer impacted issues over "all low-R² issues."
  Low-R² remains a prioritization signal, not the sole trigger.

### Why this phase

Tagging quality is not a one-time feature. The system needs ongoing feedback to
stay sharp as the workspace changes.

### Success criteria

- Missing taxonomy concepts are discovered systematically
- Regressions in tagging quality are visible quickly
- Low-R² issues are re-enriched automatically when the taxonomy improves and
  they are directly impacted by the change

## Recommended Rollout Order

Ship in this order:

1. Phase 0: benchmark and diagnostics workflow
2. Phase 1: current-pipeline fixes (stale hints, metadata preservation)
3. Phase 1.5: few-shot examples in tagging prompt
4. Phase 2: retrieval-first candidate narrowing
5. Phase 3: embedding-based verification
6. Phase 4: proposed-tag workflow
7. Phase 5: data-driven taxonomy growth
8. Phase 6: ongoing review/reporting

This order improves quality early while avoiding irreversible taxonomy churn.
Phases 1.5 and 2 can be developed in parallel since they modify different parts
of the pipeline (prompt content vs candidate selection).

## Success Metrics

Track these metrics explicitly:

- Benchmark precision / recall on expected tags
- Percentage of issues with only low-specificity tags (specificity < 0.3)
- Percentage of issues with at least one medium- or high-specificity tag
- Rate of high-relevance, low-alignment assigned tags (misclassification rate)
- Rate of residual-near existing tags that were not assigned (missed tag rate)
- Aggregate R² (how well the overall taxonomy explains the corpus)
- Reuse rate of newly created tags after 30 and 90 days
- Active catalog growth rate vs merge / rejection rate
- Tagger stability (agreement rate when the same issue is re-scored)

## Risks

### Over-constraining retrieval

A narrow shortlist can improve specificity but hurt recall if the right tag is
never shown to the model.

Mitigation:

- Include broad anchors and an evaluation fallback path
- Tune shortlist size against benchmark results
- Monitor benchmark recall explicitly alongside precision

### Taxonomy fragmentation

Pushing specificity too aggressively can create overly narrow tags that are not
reusable.

Mitigation:

- Gate promotion of new tags (Phase 4)
- Require descriptions with clear inclusion and exclusion rules
- Track reuse rate as a first-class metric

### More latency and cost

Retrieval plus verification may cost more than a single pass.

Mitigation:

- Keep retrieval local and cheap (cosine similarity over cached embeddings)
- Use LLM verification only when confidence is low, not on every issue
- Few-shot examples add prompt tokens but no additional API calls

### Self-reinforcing bad labels

Few-shot examples and automated taxonomy growth can feed current mistakes back
into the system if they rely on unvetted issue labels.

Mitigation:

- Restrict few-shot examples to verified or curated exemplars
- Run merge-before-create checks before proposing new tags
- Keep benchmark evaluation as the release gate for prompt and retrieval changes

### Product complexity

Introducing proposed tags adds workflow complexity.

Mitigation:

- Keep proposed-tag handling lightweight at first
- Default to quiet automation with explicit review surfaces only where needed

## Open Decisions

- What shortlist size gives the best precision/recall tradeoff? (Start with 15
  retrieved + 5 anchors, tune against benchmark.)
- ~~Should the few-shot examples be cached per-session or recomputed per-issue?~~
  Per-issue — the value is semantic proximity to the current issue, so caching
  across a session only helps if issues are similar. Resolved.
- Should the few-shot examples come only from a curated exemplar pool, or can
  verified live issues also qualify?
- What is the minimum evidence required to promote a proposed tag? (Start with
  ≥ 3 issues.)
- Should broad anchor tags be fixed, or derived from typed taxonomy dimensions?
- What alignment threshold should the embedding verifier use for dropping tags?
  Hard-drop thresholds should be conservative and probably tag-class-specific.
- Which benchmark set should be treated as the release gate for tagging changes?
- What relevance thresholds should evidence-grounding rules use? Initial
  picks: downrank a tag whose evidence quotes are absent from the source text
  when its claimed relevance is ≥ 0.30, and flag it when ≥ 0.60. Tune against
  the benchmark.
- Should evidence-flagged tags be displayed in the UI with their failed quote
  for human triage, or hidden entirely until reviewed?

## Recommended First Implementation Slice

If implementation starts immediately, the first slice should be:

1. Build a small benchmark set from real issues (select ~20 issues spanning the
   failure modes listed in Phase 0).
2. Fix stale hint selection (embed canonical text before calling AnnotateHints).
3. Add few-shot examples from high-R² neighbors to the tagging prompt.
4. Add retrieval-first candidate selection (replace full-catalog taxonomy with
   retrieved shortlist + anchors).

Each step should be evaluated against the benchmark before proceeding to the
next, consistent with the rollout safety requirements. In particular, step 4
should ship behind a side-by-side comparison path: run both full-catalog and
retrieval-first on each benchmark issue, compare precision and recall, and only
switch the default when retrieval-first shows clear improvement. This makes
rollout safety concrete from the first change rather than aspirational.
