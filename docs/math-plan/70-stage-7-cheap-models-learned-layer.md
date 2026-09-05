# Stage 7 — Cheap-model economics and the learned tag layer

> Strategy: shift the heavy lifting from expensive LLM opinion onto cheap
> models plus the math layer. The system already has the safety net that makes
> this safe — the verifier's evidence gate, drift diagnostics, and the ridge
> geometry that refines noisy anchors — but it has never been *used* as one.
> This stage (a) stops paying for redundant embedding calls, (b) builds the
> instrument that measures tagging fidelity per model tier, (c) routes
> enrichment cheap-first with evidence-triggered escalation, and (d) makes the
> tag matrix `T` a *learned* per-corpus artifact instead of a catalog of
> descriptor embeddings — which both closes the measured R² ≈ 0.16 gap on real
> geometry and further de-loads the LLM: the geometry corrects what a cheaper
> analyzer gets loosely right.
>
> Context claims verified against code 2026-08-07. Track-level rationale:
> math-evolution.md Track E. Queue: [README.md](./README.md).

---

## WP-701 — Embedding call hygiene: one embed per analysis

**Size:** S · No dependencies. Pure cost; no math change; no eval risk.

### Context (verified)

Every enrichment pays two identical `/embeddings` calls for the same text:

- `analyzeWithCandidateTaxonomy` embeds `raw` to build the tag shortlist
  (`internal/issueenrichment/analyze_text.go:17-21`), then calls
  `AnalyzeIssueData`, which embeds the **same text again**
  (`internal/ai/service.go:65`).
- This double ride is paid on issue create, re-enrich, combine, memory
  enrichment, and memory recall with concepts (`internal/memories/recall.go:115`
  — the comment there says "a single call rather than embedding twice", which
  is true of `recallQuerySignal` itself but not of the `AnalyzeText` path
  underneath it).
- There is **no text-keyed embedding cache anywhere**; every call hits the API.

### Design

- Thread the already-computed embedding through: give `AnalyzeIssueData` (or an
  option on it) a way to accept a precomputed `EmbeddingResult`, and have
  `analyzeWithCandidateTaxonomy` pass the shortlist embedding down. This is the
  root-cause fix — one function, all callers healed.
- Optionally (same WP, only if trivial): a small content-hash → vector LRU at
  the `Analyzer.EmbedText` boundary as belt-and-braces for repeated texts
  (re-enrich of unchanged raw). Skip if the threaded fix covers the observed
  duplication; do not build cache invalidation machinery — the hash of the text
  *is* the invalidation.

### Acceptance criteria

A test asserts one embed call per analyze (count on the stub embedder);
create / re-enrich / combine / recall-with-concepts paths exercised; matheval
baselines untouched (this must be a pure plumbing change — identical vectors,
identical results).

---

## WP-702 — Tagging-fidelity eval: the analyzer instrument

**Size:** M · No dependencies. Gates WP-703.

> **Status note (2026-08-08):** the design has been expanded into a full
> pipeline-evaluation program — see
> [../pipeline-evaluation.md](../pipeline-evaluation.md) (stage traces, replay
> ports, fixture bundles, an evaluation workbench). `AnalysisTrace` and
> `/debug/eval-tags` are the first landed pieces. That program supersedes the
> *shape* below; this WP's **exit bar is unchanged and deliberately smaller
> than the program**: a committed, reproducible per-model comparison table
> (fidelity + negation FP rate + downstream NDCG + cost) and a written floor/
> escalation budget. Ship that first; the replay/workbench machinery follows
> as its own WPs.

### Context (verified)

`internal/matheval` measures *ranking* (NDCG@8/Recall@8 over judged queries).
Nothing measures **tagging fidelity** — how well an analyzer model assigns the
fixture's tags — yet the analyzer's `r` anchors the entire math layer.
Model constants: `defaultOpenAITagModel = "gpt-5.6-terra"`,
`defaultOpenAICanonicalModel = "gpt-5.4-mini"` (`internal/ai/openai.go:18-19`),
overridable via env (`internal/ai/config.go`). The fixture corpus (48 issues,
16 tags) carries per-issue tag-domain ground truth from its generator — the
same ground truth WP-302 used to derive explore/person judgments mechanically.

### Design

An offline command (the `embedfixture` pattern: network to regenerate,
committed artifacts for CI) — `internal/matheval/cmd/tageval`:

1. Run a named analyzer model over the 48 fixture issue texts with the fixture
   taxonomy (same shortlist + prompt path production uses).
2. Score against ground truth, per model: per-issue tag precision/recall/F1;
   relevance calibration (mean relevance on correct vs incorrect assignments);
   negation false-positive rate (a negation of a ground-truth tag —
   the expensive failure per conventions §7).
3. Downstream impact: substitute the variant's `r` into the full-path matheval
   ranking run and record the NDCG@8/Recall@8 delta — tagging errors matter
   exactly as much as they move ranking, no more.
4. Cost column: tokens in/out per issue → $ per 1k enrichments, so the
   fidelity-per-dollar comparison is explicit in one table.
5. Commit the per-model result tables to `testdata/` with the generating
   command and date; the comparison table goes in the PR and in this WP's row
   on completion.

### Acceptance criteria

Committed tables for at least the current default and one cheaper candidate;
the command is reproducible with an API key; the F1 floor and escalation-rate
budget that WP-703 will enforce are *chosen and written down* here, from the
data, before WP-703 starts.

### Risks

- Fixture texts are generator-written, not real user prose — treat deltas as
  ordering evidence between models, not absolute fidelity claims (same caveat
  discipline as the synthetic fixture; the mixed-kind fixture WP-705 and any
  future real-corpus capture sharpen this).

---

## WP-703 — Cheap-first analyzer tiering with evidence-triggered escalation

**Size:** M · **Depends on:** WP-702 (the floor and budget it enforces).

### Context (verified)

Tagging runs one model for every issue regardless of difficulty. The system
already computes, per enrichment, exactly the signals that identify the hard
cases: verifier verdicts (Flag/DownRank, dominance), evidence-resolution
failures (`resolveEvidenceRanges` rejections), discarded negations, and — post
hoc — `DriftCosine` (geometry disagrees with tagging). Constraint 5
(math-evolution §2) holds: the analyzer's `r` stays load-bearing; this WP
changes who *produces* it, not its role.

### Design

- **Tiering:** first-pass tagging runs the cheapest tier that passed WP-702's
  floor. Escalation re-runs `AnalyzeIssueData` with the stronger model and
  keeps the stronger result.
- **Escalation triggers (from signals that already exist):** any verifier
  Flag/DownRank verdict; any evidence range that fails to resolve; all
  assigned relevances below a threshold (the analyzer visibly guessing); and,
  on re-enrich, `DriftCosine` below the tag-health floor. Triggers are ORed;
  each increments a per-trigger counter.
- **Budget:** a hard cap on escalation rate (from WP-702's table — the point
  is most issues never escalate). Counters + rate exposed on a debug endpoint,
  same pattern as `/debug/embedding-fallbacks`.
- **Rollout per conventions §2:** ship dark behind config (default = current
  single-model behavior), measure the escalation rate and WP-702 fidelity on
  the tiered path, flip in its own PR.
- Canonicalization (`gpt-5.4-mini`) and labeling are in scope only if WP-702's
  method extends to them cheaply; otherwise file follow-on rows. Do not tier
  the *verifier's* thresholds — they are the net, not the load.

### Acceptance criteria

Tiered path ≥ the strong-tier F1 within the WP-702-chosen margin at ≤ the
budgeted escalation rate, measured by the WP-702 command on both
configurations; flip PR references the table; single-model path remains
reachable via config.

### Risks

- Escalation loops (escalated result still trips a trigger): escalate at most
  once per enrichment, by construction.
- Cheap-tier negation quality: negations are already evidence-gated, and
  WP-702 measures the negation false-positive rate explicitly — if the cheap
  tier fails *that* specifically, disable negation emission on the cheap tier
  and let escalation carry it (negative signal stays expensive, §7).

---

## WP-704 — Learned tag matrix `T`: anchored ridge in the transposed direction

**Size:** M–L · **Depends on:** WP-301 (shipped). Independent of WP-702/703 —
runs in parallel. The single highest-leverage item in this stage.

### Context (verified)

Tag vectors are embeddings of `"name - description"`
(`internal/tags/catalog.go:374-385` `embedTag`), assembled verbatim into the
ridge basis by `ridgeTagMatrix`
(`internal/issuemath/ridge_decomposition.go:576`). `T` is **fixed**: no step
learns tag directions from the corpus. On real geometry the consequence is
measured: aggregate R² ≈ 0.16 (math-evolution §4 caveat 3) — the descriptor
subspace explains ~16% of embedding variance, so `w_F` stays near its floor
and the residual term carries ranking. The whitepaper's own note
(math-evolution §13): "if tag embeddings are noisy, `f_i` is noisy — worth its
own review." This WP is that review, resolved by learning.

### Design

**The solve.** With `E ∈ ℝ^(N×D)` (issue embeddings in the ranking regime's
geometry — uncentered unit vectors), `R ∈ ℝ^(N×K)` (signed anchors
`r⁺ − r⁻`, same `signedAnchor` semantics the ridge consumes), and `T⁰` the
descriptor-embedding rows:

```
T_L = argmin_T ‖E − R·T‖²_F + γ₀‖T − T⁰‖²_F
    = (RᵀR + γ₀I)⁻¹ (RᵀE + γ₀·T⁰)
```

The same anchored-ridge trick the shipped model already trusts, transposed:
production anchors `f` toward the analyzer's `r` with `T` fixed; this anchors
`T` toward the descriptors with the analyzer's `R` as design matrix. Closed
form, deterministic, one K×K Cholesky with D right-hand sides —
O(NK² + K³ + K²D), sub-second at N=10k/K=200/D=1536, ~2.4 MB per learned `T`.

Properties that fall out (state them as tests, not prose):

- **Cold tags are pinned automatically.** A single scalar γ₀ suffices: for a
  tag with zero observations the k-th row of `RᵀR` and `RᵀE` are zero, so
  `t_k = t⁰_k` *exactly*; lightly-used tags stay near their descriptor;
  heavily-used tags drift to their empirical meaning. No per-tag schedule
  needed — usage-dependence is built into least squares.
- **Credit assignment beats centroids.** Because all tags are solved jointly,
  `RᵀR`'s off-diagonal untangles co-occurrence: the `ios` part of a
  crash-and-iOS issue's embedding is attributed to `ios`, not smeared into
  `crash`. (The centroid arm below exists to demonstrate this on the fixtures,
  then lose.)
- Re-unit-normalize learned rows after the solve (zero rows stay zero), so λ
  scales stay comparable with the descriptor basis.
- Corpora below `MinDecompositionIssues` return `T⁰` unchanged — the standard
  degradation shape.

**Regime discipline (conventions §6, extended).** The learned `T` is a
**ranking-regime artifact only**, computed on uncentered inputs, held in its
own revision-keyed cache (`ridgelambda` shape), with GCV λ re-selected on the
learned basis (the λ cache already takes the basis — nothing new). Everything
diagnostic **keeps descriptor `T`**: drift/tag-health, the verifier's
alignment checks, specificity, the map's `Σ_tags`, and themes (centered
structure regime). This is load-bearing, not conservatism: drift works because
`T` is an *independent witness* — the analyzer says `r`, the geometry says
`f`, disagreement is signal. A witness fit *from* `r` is coached: a
systematically mis-tagged tag would drag its learned vector toward the
mis-tagging and drift would go blind to it. Same "looks like a bug, is
policy" note as the two-λ regime — record it in the deviations ledger.
Re-litigating any diagnostic consumer onto learned `T` is its own future WP
with its own eval.

**The study (config-matrix pattern, rule pre-committed before measurement).**
Arms: descriptor `T` (status quo) / relevance-weighted centroids / learned
`T_L` at γ₀ swept over a small grid. Both fixtures, full-path where the seam
exists. Qualification: must beat the shipped default on the **real** fixture
beyond +0.01 NDCG@8 and not lose > 0.02 NDCG@8 on synthetic; explore and
person runs corroborate the finalist. The synthetic fixture builds embeddings
from descriptor-tag sums, so it structurally flatters the *descriptor* basis —
for once the circularity cuts against the new thing; a real-fixture win is the
load-bearing evidence, per conventions §3.

**Honest R².** In-sample R² on a basis fit to minimize residuals is
self-fulfilling, and R² feeds `w_F`. Report cross-fitted aggregate R²
(k-fold: learn `T_L` on k−1 folds, pool residuals out-of-fold) next to
in-sample in the study; if they diverge materially, `w_F` uses the
cross-fitted value. Cheap given the closed form.

**Observability.** A debug surface listing per-tag `cos(t_k, t⁰_k)` sorted
ascending — "which tags' usage has drifted from their description". This is a
better staleness detector than anything the catalog has today; future curation
hook, file the follow-on when it proves out.

### Implementation steps

1. Pure function in `internal/issuemath` (learned-basis solve + normalization
   + guards) with a bit-for-bit determinism test and the exactness test for
   unused tags.
2. Study: extend the config-matrix test with the three arms; commit numbers;
   decision recorded here and in the deviations ledger.
3. Wire (only if qualified): revision-keyed cache; ranking surfaces consume
   the learned basis through the existing `ridgedecomp` bundle path; descriptor
   basis remains the fallback at every degradation point. Flip in its own PR
   with updated golden baselines.

### Acceptance criteria

Study table committed with the pre-committed rule stated in the PR; if
qualified: golden baselines updated, degradation tests (tiny corpus → `T⁰`;
missing descriptor rows), regime separation asserted (diagnostics provably on
descriptor `T`), determinism test green. If not qualified: numbers and verdict
recorded here, code stays dark — that outcome is acceptable and cheap.

### Risks

- **Witness coaching** — retired by the regime split above.
- **R² self-inflation** — retired by cross-fitting + NDCG as the gate.
- **K ≥ D exposure unchanged** (K doesn't grow), but note the interaction
  for WP-606: a learned basis makes small-D embedding models *more* viable,
which is exactly when the GCV cliff needs its answer.

### Study record (2026-08-08)

The pre-committed qualification rule is: **learned `T_L` qualifies only if it
beats the shipped default on the REAL fixture by more than +0.01 NDCG@8 and
loses no more than 0.02 NDCG@8 on synthetic.** The synthetic fixture builds
embeddings from descriptor-tag sums and structurally flatters the descriptor
basis; its result is a floor, while the real-fixture result is load-bearing.

`go test ./internal/matheval -run TestLearnedTagBasisStudy -learnedbasis -v`
evaluates all rows through the full search-ranking seam, with GCV λ selected
again for each basis. Cross-fitted R² pools out-of-fold residuals over five
deterministic folds; it is reported beside in-sample R² because the latter is
self-fulfilling for a learned basis.

| Fixture | Arm | γ₀ | GCV λ | NDCG@8 | Recall@8 | R² in-sample | R² cross-fitted |
|---|---|---:|---:|---:|---:|---:|---:|
| synthetic | descriptor `T` | — | 1.000 | 0.9366 | 0.9560 | 0.9005 | 0.9005 |
| synthetic | relevance-weighted centroids | — | 1.000 | 0.9471 | 0.9560 | 0.9045 | 0.8825 |
| synthetic | learned `T_L` | 0.1 | 1.000 | 0.9332 | 0.9638 | 0.9380 | 0.9030 |
| synthetic | learned `T_L` | 1 | 1.000 | 0.9369 | 0.9622 | 0.9314 | 0.9038 |
| synthetic | learned `T_L` | 5 | 1.000 | 0.9388 | 0.9622 | 0.9199 | 0.9099 |
| synthetic | learned `T_L` | 20 | 1.000 | 0.9373 | 0.9560 | 0.9093 | 0.9052 |
| real | descriptor `T` (shipped default) | — | 0.300 | 0.9399 | 0.9195 | 0.1729 | 0.1729 |
| real | relevance-weighted centroids | — | 0.010 | 0.8449 | 0.6371 | 0.5786 | 0.2897 |
| real | learned `T_L` | 0.1 | 0.100 | 0.9563 | 0.9536 | 0.6032 | 0.3010 |
| real | learned `T_L` | 1 | 0.300 | 0.9619 | 0.9544 | 0.5393 | 0.3083 |
| real | learned `T_L` | 5 | 0.300 | 0.9592 | 0.9466 | 0.3611 | 0.2577 |
| real | learned `T_L` | 20 | 0.300 | 0.9594 | 0.9320 | 0.2333 | 0.2006 |

**Verdict: qualified for a separate, dark wiring WP.** `γ₀=1` is the finalist:
its real-fixture NDCG@8 gain is +0.0220 and its synthetic delta is +0.0003.
No runtime consumer changes here: ranking, drift, verifier alignment,
specificity, map, and themes continue using descriptor `T` until that separate
WP has its own cache, seam, and rollout review.

---

## WP-705 — Mixed-kind fixture: documents, ideas, and tasks

**Size:** M · **Depends on:** WP-301 machinery (shipped). Sharpens WP-704;
gates any generalization claim.

### Context

Every fixture is software-issue text in one register. The generalization
direction (documents & ideas alongside tasks) changes the geometry in a
specific way: a doc, a task, and an idea about the *same* initiative differ in
text register, so their raw embeddings cosine lower than two same-kind texts —
depressing cross-kind similarity systematically and biasing every fixed
threshold tuned on one register. Cross-kind ranking is also exactly where a
learned `T` should win by the widest margin (a learned tag direction spans the
registers its tagged items actually exhibit; a one-line descriptor embedding
does not). Today that claim is unmeasurable.

### Design

- Extend the fixture generator with per-kind register templates (doc / idea /
  task renderings of the same tag-domain content), keeping the tag-domain
  ground truth so WP-702's mechanics reuse unchanged.
- Judged queries where the relevant set deliberately spans kinds; re-embed via
  `embedfixture`; add baseline entries under a `mixed.` prefix alongside
  `synthetic.`/`real.`.
- Record the cross-kind vs same-kind cosine gap as a fixture statistic — it is
  the number that justifies (or kills) per-kind centering means and
  threshold re-derivation as follow-on WPs. File those as rows here when the
  data exists; do not build them speculatively.

### Acceptance criteria

Mixed fixture committed with real-model embeddings; baseline rows asserted on
ordinary `go test`; WP-704's study re-run on it with the delta recorded (this
is the fixture where the learned basis must show its cross-register value).

---

## WP-706 — Embedding-model qualification harness and migration checklist

**Size:** S–M · **Depends on:** WP-606 before any small-D candidate is taken
seriously. Optional lane — embeddings are already the cheap call; this exists
for the local-first/off-API motivation, not cost.

### Context (verified)

The embedding model is a constant (`internal/ai/openai.go:20`) with an env
override, but nothing can *qualify* a replacement candidate. The dimension is pinned
operationally: all five HNSW indexes are partial on `vector_dims = 1536`
(`internal/issues/sqlc/schema.sql` — issues, tags, memories, and both
projection tables), and the SQLite store's vec0 table is `FLOAT[1536]`
(`internal/issues/sqlitemigrations/000003_issue_embedding_vectors.up.sql`).
Graceful mixed-dimension paths already exist (dimension-mismatched vectors
score as pure semantic similarity; SQLite drops them to in-Go cosine).

### Design

1. Parameterize `embedfixture` by model + dimension so any candidate produces
   a fixture variant; qualification = the WP-304 rule against the shipped
   default on the real (and, once it exists, mixed) fixture.
2. Write the migration checklist as part of this WP's doc change, not code:
   new HNSW indexes + backfill order, sqlite-vec table migration, the
   mixed-dim grace window semantics during rollout, and **WP-606 resolved
   first** for any D within a small multiple of catalog K.
3. No swap in this WP. The deliverable is the harness plus a written decision
   procedure; an actual candidate gets its own WP with its own numbers.

### Acceptance criteria

A second model's fixture can be produced and evaluated with one command; the
checklist is in this file; no production behavior changed.

---

## Sequencing within the stage

```
WP-701 (call hygiene) ── standalone, first: pays for the rest
WP-702 (tagging eval) ──► WP-703 (cheap-first tiering + flip)
WP-704 (learned T) ───── parallel with 702/703; study → wire → flip
WP-705 (mixed fixture) ─ after 704's first study; re-runs it
WP-706 (embed harness) ─ opportunistic; WP-606 gates small-D candidates
```

WP-704 and the 702→703 lane are independent by design: one attacks the
geometry side (make the math extract more from whatever the models produce),
the other the economics side (produce it cheaper). They compound — a learned
basis that corrects noisy anchors is precisely what makes a cheaper analyzer
safe — but neither waits on the other.
