# Math Evaluation Harness

`internal/matheval` is the offline evaluation harness for Sortit's scoring
and ranking math — the answer to [white paper](./whitepaper.md) §10.2 item 9
("hyperparameters everywhere with no evaluation harness"). It makes changes
to `internal/scoring/constants.go`, `internal/issuemath`, and the search
ranking in `internal/map` measurable instead of hand-felt.

It runs as a normal `go test`, needs no network, database, or AI services,
and is part of the regular CI test run.

## Running it

```bash
# Run the harness and print the metrics
go test ./internal/matheval -run TestMathEval -v

# After an intentional math change: re-record the golden files
go test ./internal/matheval -update

# Hyperparameter sweep reports (log-only, opt-in)
go test ./internal/matheval -run TestSweepFactorWeight -sweep -v

# WP-304 ridge config matrix: 6 similarity configs × 2 fixtures (log-only, opt-in)
go test ./internal/matheval -run TestRidgeConfigMatrix -configmatrix -v

# Centered-regime shadow comparison + λ sweeps (log-only, opt-in)
go test ./internal/matheval -run TestRidgeShadowComparison -ridge -v
```

A run evaluates **two ranking profiles** (per fixture) and produces output like:

```
[synthetic/rank1] search: queries=32 NDCG@8=0.8658 Recall@8=0.9117
[synthetic/rank1] factor model: factorWeight=0.6230 aggregateR2=0.6230
[synthetic/rank1] per-issue R²: n=48 mean=0.6230 median=0.6487 p10=0.4175 p90=0.8553
[synthetic/ridge] search: queries=32 NDCG@8=0.9366 Recall@8=0.9560
[synthetic/ridge] factor model: factorWeight=0.9005 aggregateR2=0.9005
[synthetic/ridge] per-issue R²: n=48 mean=0.9005 median=0.9272 p10=0.8572 p90=0.9523
[synthetic] ridge GCV λ_unscored = 1.0000
```

and compares both against the keyed baseline in
`internal/matheval/testdata/baseline.json`:

- **`rank1`** — search with no options: the fallback path production takes
  when the ridge decomposition cache yields nothing (small/degenerate
  corpora).
- **`ridge`** — the shipped default (WP-304): the harness injects the
  **uncentered** full-corpus bundle via `WithRidgeDecomposition` at the GCV λ
  selected on the same uncentered inputs, mirroring how the API layer injects
  the uncentered `ridgedecomp.Cache`. The GCV λ is re-derived on every run and
  recorded in the baseline (`gcvLambdaUnscored`) for observability only —
  it is never asserted, so a grid or GCV change surfaces as a metric delta
  rather than silent drift.

Both profiles are asserted on every ordinary `go test` run with no opt-in
flag, so a change that regresses either the production ridge blend or the
rank-1 fallback fails CI.

## What is measured

### Search ranking: NDCG@8 and Recall@8

For each of the 32 labeled queries, the harness drives
`issuemap.SearchFromQueryWithTags` end-to-end — the same entry point the
search API uses, including the factor/residual blend, freshness and velocity
modifiers, authority, specificity penalties, and co-occurrence boosts — and
scores the returned top 8 (the default result limit) against hand-labeled
graded judgments:

- **NDCG@8** (normalized discounted cumulative gain) rewards putting the
  *most* relevant issues at the *top*. Gains are exponential in the grade
  (0–3), positions are log-discounted. 1.0 means the ranking matches the
  ideal ordering of the labels.
- **Recall@8** is the share of all relevant issues (grade > 0) that appear
  anywhere in the top 8. It ignores ordering and answers "did we find it at
  all?".

Both are averaged over all queries. Watch NDCG when changing weights that
reorder results (blend weights, boosts, penalties); watch Recall when
changing anything that can push relevant issues out of the window entirely.

### Factor model: per-issue R² distribution

The harness runs each profile's decomposition over the corpus —
`issuemath.ComputeFactorDecomposition` on centered inputs for the `rank1`
profile, the uncentered full-corpus ridge bundle at its GCV λ for `ridge` — and
summarizes the per-issue R² (the share of each embedding's variance explained
by its tag loadings) as mean / median / p10 / p90, plus the resulting
data-driven `factorWeight` and `aggregateR2`. These are *descriptive*
fingerprints of the decomposition math rather than quality scores — higher is
not automatically better — so the harness flags drift in **either**
direction beyond a small tolerance.

## Pass/fail semantics

Applied independently to each baseline profile (`rank1` and `ridge`):

- **NDCG@8 / Recall@8**: the test fails if a metric drops more than 0.005
  below the baseline. Improvements pass and log a suggestion to ratchet the
  baseline up with `-update`.
- **R² distribution, factorWeight, aggregateR2**: the test fails if any
  value moves more than 0.02 in either direction, forcing intentional math
  changes to be acknowledged by re-recording the baseline.

When a regression is intentional (e.g. a trade-off that helps a metric the
harness doesn't capture), re-record with `-update` and explain the movement
in the PR description.

## The fixture corpus

`internal/matheval/testdata/corpus.json` holds 48 synthetic issues, 16 tags,
and 32 queries. It is generated deterministically by
`GenerateCorpus` (`internal/matheval/generate.go`) and pinned by a test, so
it cannot drift from its documented provenance:

- **Tag embeddings** are hash-derived unit vectors (24 dimensions)
  correlated within a domain — `billing`≈`payments`, `auth`≈`login`,
  `export`≈`pdf` — with `ui` and `backend` as low-specificity generic
  buckets.
- **Issue embeddings** are the relevance-weighted sum of the issue's tag
  directions plus seeded hash noise. The noise share varies per issue
  (0.3–1.0), producing a realistic spread of per-issue R², including three
  untagged "off-taxonomy" notes that decompose to pure residual.
- **Every vector shares a corpus-wide common direction** (anisotropy),
  mirroring real text-embedding models where unrelated same-corpus texts
  still cosine at ~0.5–0.7 (whitepaper §2.0). The mean raw pairwise cosine
  across fixture issues is ~0.58 and drops to ~0 after corpus-mean
  centering; `TestGeneratedCorpusIsAnisotropic` pins both bounds. Without
  this property the harness would not exercise the runtime's centering
  path, and constants tuned here would be tuned against an unrealistically
  isotropic geometry.
- **Queries** are built the same way from the analyzer-style tag scores a
  real query would carry; a few are bare tag names (`billing`, `ui`,
  `backend`) to exercise the tag-correlation nudge and the generic-tag
  co-occurrence paths.

No OpenAI calls are involved anywhere; everything is reproducible from the
generator. Issue ages are uniform so freshness weighting never makes the
metrics depend on the wall clock.

## The judgment set

`internal/matheval/testdata/judgments.json` is the hand-labeled half: for
each query, a map of issue ID → grade (3 = the issue the query is about,
2 = same topic cluster, 1 = marginally related). The loader validates both
directions — every judgment must reference a real query and real issues, and
every corpus query must be labeled — so the two files cannot silently
diverge.

When adding queries or issues, extend the specs in `generate.go`, label the
new queries in `judgments.json`, then run `go test ./internal/matheval
-update` to regenerate the corpus and baseline together.

## Sweep mode

`TestSweepFactorWeight` (opt-in via `-sweep -v`) reports NDCG@8/Recall@8 as
a function of the factor share `w_F` of the similarity blend, forced through
`issuemap.WithFactorWeightOverride`, alongside the data-driven weight's
metrics. The production blend identifies `w_F` with the decomposition's
aggregate R² — a variance-explained quantity, not a ranking-utility one —
and this sweep is the evidence for whether that identification holds: on the
current fixtures the NDCG curve plateaus over `w_F ∈ [0.55, 0.90]` and the
data-driven weight (0.62 plus the per-query tag-correlation nudge) sits in
that plateau, slightly above the best fixed override. Sweeps are reports,
not gates — they never fail on metric values.

## What this harness does not cover

- A real *corpus*: the `real` fixture (WP-301) covers real embedding geometry
  — the same texts re-embedded with production `text-embedding-3-small`,
  committed so CI needs no network — but the texts themselves were generated
  from the tag structure (layer 1). Absolute metric values are still not
  comparable to production quality; cite the `real` rows for geometry claims
  (conventions §3) and treat synthetic deltas as floor arguments.
- The AI analyzer (tag assignment quality) and the enrichment verifier.
- Region-aware re-ranking options (`WithRegionTarget`, `WithAntiCorrelators`)
  — a natural next extension of the same corpus.

Explore and person-recommendation scoring **are** covered as of WP-302: 12
explore seeds and 6 synthetic person histories (over the existing 48-issue
corpus), graded mechanically from the generator's tag-domain ground truth, are
evaluated on both models and both fixtures and asserted under the `explore` /
`person` baseline keys. The judgment files (`testdata/explore_judgments.json`,
`testdata/people.json`) are pinned to their derivation by
`TestExplorePersonFixturesMatchDerivation` and regenerate with `-update`.
