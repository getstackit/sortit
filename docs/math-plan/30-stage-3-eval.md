# Stage 3 — Evaluation expansion

> Goal: the harness stops flattering us. Three coverage gaps, three WPs.
> Can interleave with Stage 2 (no shared code). Queue: [README.md](./README.md).

**What exists today (verified):** `internal/matheval` — synthetic corpus of
16 tags / 48 issues / 32 fully-judged queries at embedding dim 24, NDCG@8 and
Recall@8, a golden baseline with regression assertions, and R² distribution
tracking. Known limits, all confirmed against code: fixture embeddings are
generated as relevance-weighted tag sums (+ anisotropy ~0.55 + noise), which
structurally favors tag-space methods; there is zero coverage for explore,
person recommendation, and map projection; production embeddings are 1536-d
`text-embedding-3-small` while the fixture is 24-d.

---

## WP-301 — Real-embedding fixture

**Size:** M · **Depends on:** nothing.

### Why

Every ridge-vs-rank-1 number we have is a *floor argument*: on embeddings
literally constructed from tags, the tag-space model wins by construction.
The shadow harness says so itself. Until a fixture with real embedding
geometry exists, no fixture delta should change a product decision on its
own. This WP is what makes the harness's verdicts transferable.

### Design

Two-layer approach, cheapest first:

1. **Real-model embeddings over the existing fixture texts.** The fixture
   issues have generated titles/bodies; embed them once with
   `text-embedding-3-small` (the production model, `internal/ai/openai.go`),
   store the 1536-d vectors in `testdata/` (48 × 1536 floats ≈ 300 KB as
   compact JSON/binary — fine to commit), and re-embed the 16 tag
   descriptions the same way. The judgments transfer unchanged (they judge
   query↔issue relevance, not vectors).
2. **If layer 1's texts prove too synthetic** (metrics saturate, or the tag
   structure still dominates because the texts were generated *from* tags):
   curate a small real corpus — ~50 anonymized issues from this repo's own
   Sortit instance, exported, scrubbed, and re-judged (32 queries, same
   grading rubric as `judgments.json`). This is a day of curation work;
   only spend it if layer 1 fails to discriminate.

Mechanics:

- A one-off generation tool (`go run ./internal/matheval/cmd/embedfixture`,
  or a flagged test like the existing sweep patterns) that calls the real
  API; requires `OPENAI_API_KEY`; output committed so CI never needs the key.
- The harness gains a fixture dimension: every eval and baseline entry runs
  `synthetic` and, where present, `real`. Baselines keyed accordingly
  (extends the WP-101 keyed-baseline shape).
- Re-run the full comparison table on the real fixture: rank-1 vs ridge
  (GCV), tag-space vs reconstruction-space, centering on/off. **Publish the
  results in math-evolution §4** — this either confirms the ridge win on real
  geometry or is the most important negative result the program can produce.

### Validation

CI green with committed vectors and no network; the comparison table
reproduced by a documented command.

### Acceptance criteria

- A real-geometry fixture in `testdata/`, regeneration documented.
- Both baselines asserted on both fixtures.
- math-evolution §4 rewritten: the "floor argument" caveat replaced by real
  numbers, whichever direction they point.

### Risks

- Real embeddings may shrink the ridge margin substantially. That is a
  *result*, not a failure — but pre-commit to the interpretation: if ridge
  does not beat rank-1 on real geometry within noise, the default flip gets
  re-examined (its cost is complexity, and complexity needs a payoff), and
  this plan's Stages 4–5 still stand (they consume `f_i` for structure, not
  for ranking wins).
- Committed-vector staleness when the embedding model changes: record the
  model name + date in the fixture metadata; regeneration is one command.

---

## WP-302 — Explore and person-recommendation eval coverage

**Size:** M · **Depends on:** nothing (better after WP-101's keyed baselines).

### Why

Explore and person recommendations inherited the ridge default with zero
harness coverage — the math-evolution status board has carried the caveat
"only search is harness-validated" since the flip. They share the blend code
but not the *query shape*: explore is seeded by an issue (not a query
embedding), and person fit is seeded by a profile. Different failure modes,
unmeasured.

### Design

**Explore eval:**

- Judgments: for each of ~12 seed issues in the fixture corpus, grade the
  other 47 issues for "would a user exploring from this seed want to see
  this" (0–3, same rubric family as search). Derive candidate grades
  mechanically where possible (shared region/tags from the corpus generator's
  ground truth) then hand-adjust — the corpus generator knows which issues it
  made related.
- Metric: NDCG@8 / Recall@8 per seed, aggregated like search.
- Runner: call `ExploreFromIssuesWithTags` through the same option-injection
  shape production uses (with/without `WithExploreRidgeSimilarity`) — both
  paths baselined.
- Explore-specific modifiers (relationship boost, freshness^0.5) stay ON —
  this is a full-path eval; the similarity-only comparison exists already in
  the ridge shadow test.

**Person-recommendation eval:**

- Fixture extension: assign ~6 synthetic people histories (sets of resolved
  issues drawn from the corpus's ground-truth regions, 5–15 each), leaving
  known-region open issues as the candidate pool.
- Judgment: an open issue is relevant to a person in proportion to region
  overlap with their history (mechanical grades from generator ground truth;
  spot-check by hand).
- Metric: NDCG@8 over `recommendOpenIssues` output per person.
- This fixture is also the substrate WP-401 (profiles on `f_i`) will use to
  compare profile constructions — build it once here.

### Steps

1. Corpus generator emits ground-truth relatedness and person histories
   (extend `generate.go`; regenerate `corpus.json` + new judgment files;
   the generator's determinism keeps this reviewable).
2. Explore runner + baseline entries (`explore.rank1`, `explore.ridge`).
3. Person runner + baseline entries (`person.rank1`, `person.ridge`).
4. Retire the "only search is harness-validated" caveat in
   math-evolution §6 / whitepaper.

### Acceptance criteria

Explore and person paths regression-guarded in CI on both model paths;
the caveat removed from the docs; ridge-vs-rank-1 deltas for both surfaces
recorded in math-evolution §4's table.

### Risks

Mechanical judgments from generator ground truth are circular in the same way
the synthetic embeddings are — they grade "same region" not "actually
useful". Acceptable for a *regression* guard (detects change); pair with
WP-301's real fixture for calibration where possible, and say so in the
judgment files' headers.

---

## WP-303 — Map-projection quality metric

**Size:** M · **Depends on:** nothing. **Gates Stage 5.**

### Why

The map has zero eval coverage (verified: `matheval/metrics.go` has only
NDCG/Recall/Distribution). Stage 5 proposes replacing the projection's input
entirely (tag-relevance PCA → theme weights). Without a metric, that flip
would be judged by eyeball. The metric must exist and be baselined on the
*current* projection well before Stage 5 starts, so "did the new map get
worse" has an answer.

### Design

Three complementary measurements, all computable from fixture ground truth
plus the projection output (`ComputePositionsAligned`):

1. **Neighborhood preservation** — trustworthiness/continuity (or the
   simpler symmetric kNN-overlap): for k ∈ {5, 10}, the mean overlap between
   each issue's k nearest neighbors in 2-D layout space vs in the reference
   space. Two reference spaces, reported separately: centered embedding
   cosine (does the map respect semantics?) and the corpus ground-truth
   region assignment (does the map respect the generator's intended
   clusters?).
2. **Cluster legibility** — silhouette of the ground-truth regions under the
   2-D layout. (The repo already has silhouette code in `internal/map`'s
   clustering; reuse or mirror it.)
3. **Refresh stability** — apply the standard mutation sequence (same one
   WP-203 uses) and measure mean per-issue displacement after
   Procrustes-aligned refreshes. The current projection's Procrustes chain
   should score well here; this is the guard that keeps any successor honest
   about continuity, which is the property users actually feel.

Baseline all three for the current PCA-on-X·Σ projection; assert with
tolerances like the ranking baselines.

### Steps

1. Metrics in `internal/matheval` (pure functions over positions + reference
   data), unit tested on hand-built configurations (perfect grid, shuffled,
   collapsed).
2. Projection runner: corpus → `ComputePositionsAligned` → metrics; wire the
   mutation-sequence stability run.
3. Baseline entries (`map.neighborhood`, `map.silhouette`,
   `map.stability`); document each metric's meaning and failure smell in
   [../math-eval.md](../math-eval.md).

### Acceptance criteria

Current projection baselined on all three metrics in CI; math-eval.md's
"not covered: map" gap closed; Stage 5's entry criterion is satisfiable.

### Risks

Metric choice can bake in the current projection's biases (it was tuned by
eyeball on these fixtures). Mitigate by keeping the three measurements
separate rather than collapsing to one score — Stage 5 argues against each
individually.
