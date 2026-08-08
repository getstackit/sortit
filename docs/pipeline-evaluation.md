# Document Pipeline Evaluation

Sortit can already evaluate math offline (`internal/matheval`) and tag outcomes
against a small debug fixture (`/debug/eval-tags`). This document defines the
next layer: a reproducible evaluation of the whole document-onboarding path,
from input through quotations and corpus math.

## Target shape

```text
document
  -> canonical text -> embedding -> candidate retrieval -> context assembly
  -> model classification -> score normalization -> verification / quotations
  -> persistence -> corpus projections -> search, map, themes, and R²
```

Each arrow must be independently replayable and each stage must return a named
artifact. A pipeline run is therefore both a production result and an evaluation
record, rather than a collection of log lines.

`issueenrichment.AnalysisTrace` is the first implementation of this boundary.
It is returned by `AnalyzeText` and the authenticated debug endpoint. It records
candidate-source counts, context counts, pre-verifier model output, floor and
attenuation effects, verification verdicts, grounded-quotation count, and model
metadata already present in the response. It does not expose prompt or retrieved
memory bodies.

## Refactoring sequence

1. **Make enrichment a composed pipeline.** Extract the private work inside
   `IssueEnricher` into small stage interfaces with immutable input/output
   structs:

   - `TextCanonicalizer`
   - `Embedder`
   - `CandidateSelector`
   - `ContextAssembler` (few-shot examples, project frame, prior decisions)
   - `TagClassifier`
   - `ScorePostProcessor` (floor and generic attenuation)
   - `TagVerifier` (alignment, dominance, negation, evidence resolution)

   `IssueEnricher` remains the production orchestrator. `AnalyzePersistedIssue`
   owns canonicalization and persistence preparation; a pure `Run` owns all
   enrichment stages. Do not put database handles or HTTP clients in stage
   inputs.

2. **Give every run an identity.** Add `PipelineVersion` (source revision plus
   prompt/schema versions), model IDs, candidate-mode/config values, timings,
   token/cost estimates, and artifact hashes to the trace. Persist a compact
   trace with `issue_enrichment_events`; store full prompt/completion payloads
   only in an explicitly enabled, access-controlled evaluation artifact store.

3. **Add replay ports.** Production adapters call OpenAI and Postgres. Test
   adapters load captured embeddings, candidate catalogs, model structured
   responses, and memory retrievals from fixture files. A replay must make zero
   network calls and be byte-for-byte deterministic.

4. **Separate document quality from corpus quality.** A document evaluator
   stops after verification. A corpus evaluator applies a declared batch of
   document results to an isolated fixture store, then measures specificity,
   co-occurrence, centering, ridge/factor decomposition, search, themes, and
   map quality. This prevents a changed tagger from being accidentally judged
   only by a stale corpus projection.

5. **Create one evaluation entry point.** `sortit eval pipeline` should accept
   a fixture bundle and emit a versioned JSON report. CI runs replay fixtures;
   a manually triggered, budgeted live mode can refresh recordings or measure a
   candidate model. Baselines must only be updated through an explicit
   `-update` flag, following `internal/matheval`'s model.

## Fixture bundle

Keep it in version control, for example `internal/pipelineeval/testdata/<name>/`:

```text
manifest.json          pipeline/config versions and fixture provenance
documents.jsonl        source document, expected tags, expected negations
catalog.json           tags, descriptions, embeddings, specificity
context.json           concepts, memories, few-shot selections
recordings.jsonl       embedding and structured-classifier replay responses
judgments.json         candidate relevance, tag grades, quotation spans
corpus-baseline.json   expected corpus-level metrics
```

Use three fixture classes:

- **Synthetic:** dense, adversarial, deterministic edge cases (negation,
  candidate omission, generic drag, invalid quotation, chunked embeddings).
- **Curated project corpus:** de-identified, hand-judged real documents with
  architectural concepts and known terminology.
- **Regression corpus:** every production incident reduced to the smallest
  document and expected stage behavior that would have caught it.

Expected tags should be graded (required / relevant / acceptable alternate), not
only exact sets. Quotations are judged as byte offsets into the source text;
this avoids scoring paraphrase as evidence.

## Measures and gates

| Boundary | Report | CI gate |
| --- | --- | --- |
| Retrieval | candidate recall@K for required tags, source mix, hint precision | no recall regression |
| Classification | micro/macro precision, recall, F1; per-tag confusion matrix; stability across repeated live runs | recall and macro-F1 floors |
| Quotations | span validity, support coverage, unsupported-quote rate | zero invalid spans; coverage floor |
| Verification | kept/down-ranked/flagged confusion versus review labels; generic attenuation rate | no regression on false-positive suppression |
| Performance | per-stage latency, tokens, estimated cost, failures/retries | budget and p95 ceilings |
| Taxonomy/corpus | adoption of seeded concepts, specificity, tag entropy, residual-miss rate | report and review initially |
| Math | existing NDCG/Recall, R² distribution, exploration/person/map baselines | reuse `matheval` gates |

Do not treat aggregate R² as an absolute quality target. It is a lagging,
breadth-limited diagnostic; the document-level judgments and candidate recall
are the leading controls.

## Visual workbench

Extend the existing authenticated `/debug` page into an **Evaluation
Workbench**, backed by saved reports rather than browser-only state.

- **Document run stepper:** one column per stage, showing input/output counts,
  duration, artifact IDs, and failures. The current Pipeline trace card is the
  first slice of this.
- **Baseline vs candidate diff:** side-by-side candidate sets, raw model tags,
  post-processed tags, verifier decisions, and highlighted evidence spans.
  This is the most useful daily debugging surface.
- **Corpus quality dashboard:** metric cards with baseline deltas, an error
  table filterable by stage/tag/category, per-tag precision/recall bars, and a
  candidate-recall funnel. These need ordinary tables/bars, not a new charting
  platform.
- **Math explorer:** R² histogram, issue R² vs. tag specificity scatter,
  tag co-occurrence heatmap, and a rank-explanation waterfall for an issue or
  search result. Link points back to their document trace.
- **Regression timeline:** line charts of the gated metrics by pipeline version
  and model. Use this to distinguish a prompt/model change from a math change.

For implementation, keep the existing Tailwind/Radix UI primitives and add a
small React chart library only when the histogram/scatter/line charts ship
(Recharts is sufficient). Keep the existing custom canvas implementation for
the map; do not force it through a general charting library. Build tables and
diff views first—they answer most evaluation questions better than a dashboard.

## Delivery order

1. Ship trace fields and the Debug trace card. **Done.**
2. Extract pure stage contracts and record per-stage timing/version metadata.
3. Build replay fixtures plus `sortit eval pipeline`; gate retrieval,
   classification, and quotation validity in CI.
4. Add isolated corpus replay and join its outputs to `matheval` reports.
5. Save reports and build the comparison/dashboard views.

## Decisions intentionally deferred

- Whether full prompt/completion retention is permitted, and its retention
  policy.
- Which real documents may be committed versus only recorded in a protected
  artifact store.
- The human-review workflow for labeling acceptable alternate tags and evidence
  spans.
- Budget, cadence, and approval policy for live-model evaluation runs.
