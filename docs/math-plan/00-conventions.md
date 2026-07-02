# Math Program: Working Conventions

> How to execute any work package (WP) in this plan. These conventions are the
> distillation of what made the ridge default-flip safe; every WP in
> [README.md](./README.md) assumes them. Read once, then work the queue.

## 1. One work package = one small stack

Every WP is scoped to be a single stacked-PR unit (or a short stack of 2–4
focused branches when the WP says so). Follow the repo's stacked workflow:

```bash
git add -A
stackit create -m "feat(mathplan): WP-XXX short description"
# iterate; route fixes to the branch that introduced them (stackit absorb / modify)
mise run check          # full gate — mandatory before submit
stackit submit
```

Never submit red. In a stack, fix failures in the branch that introduced them
so the fix propagates up via restack (see `.claude/rules/stackit-workflow.md`).

## 2. Ship dark, measure, then flip

Any change that can alter ranking, layout, or user-visible aggregates follows
the three-step discipline:

1. **Dark / debug-tier first.** New math lands computed-but-unconsumed, or
   behind a `/debug/*` endpoint. No product surface changes in the same PR as
   new math.
2. **Measure.** A matheval metric (existing or added by the WP) must exist
   *before* the flip. If the WP's change is not observable by the harness, the
   WP includes extending the harness — that is not optional scope.
3. **Flip in its own PR.** The flip PR is small, references the measurement,
   and keeps the previous behavior reachable (fallback path or debug flag)
   until a soak period passes.

The ridge rollout (shadow endpoint → shadow harness → opt-in blend → default
flip with rank-1 fallback) is the template. The one gap it left — the golden
baseline never followed the flip — is WP-101, which is why it is first in the
queue.

## 3. Numbers come from the harness, not from vibes

- Hyperparameters (λ grids, thresholds, K, iteration counts) are either
  data-driven (GCV-style, no labels needed), harness-tuned (fixtures +
  judgments), or explicitly documented as hand-set with a pointer to the WP
  that will calibrate them.
- When a WP changes any metric-bearing code path, it must re-run
  `internal/matheval` and commit updated baselines in the same stack, with the
  before/after numbers in the PR description.
- Fixture caveat: the harness now runs two fixtures. The **synthetic** corpus
  generates embeddings from tag sums and structurally favors tag-space methods —
  its deltas are floor arguments, never cite one without this caveat. The
  **real** fixture (WP-301, shipped) re-embeds the same texts with production
  `text-embedding-3-small`; cite it for any claim about real geometry. Its
  headline result: the ridge tag-space ranking win *inverts* on real embeddings
  (rank-1 0.905 vs ridge 0.754 NDCG@8, GCV λ falls to the 0.01 grid floor, tag
  R² collapses to ~0.16) — see math-evolution §4 caveat 3. A fixture number
  without its fixture name (`synthetic`/`real`) is meaningless.

## 4. Determinism is a feature

Every piece of the math layer is deterministic today (stride sampling, NNDSVD,
sign conventions, tie-breaks by ID/name). Keep it that way:

- No RNG in math code. If an algorithm wants randomness (initialization,
  sampling), derive a deterministic seed from stable inputs and document it.
- Iteration order over maps must be sorted before it affects any output.
- Tests should be able to assert bit-for-bit reproducibility, and new math
  packages should include such a test (see `issuethemes` determinism test as
  the pattern).

## 5. Caches follow the revision pattern

There is exactly one invalidation mechanism in the math layer: the corpus
revision counter. New caches (theme cache, decomposition cache) must:

- Key on the revision from the shared `RevisionSource`, recompute read-through
  on mismatch or zero revision.
- Center any embedding inputs with the **revision-cached corpus means** — never
  with means computed over the cache's own sample when the shared cache is
  available.
- Return an explicit "not available" signal (the `(value, ok)` shape) that
  callers treat as "fall back", never as an error. Degradation is graceful by
  construction.

`internal/ridgelambda/cache.go` is the reference implementation; copy its
shape, including the deterministic stride sampling if the computation needs
bounding.

## 6. Two-λ awareness

Ranking and diagnostics deliberately run the anchored ridge with different
unscored penalties (GCV-selected for ranking; fixed loose for drift, so
missing-tag candidates can surface). Any WP touching ridge call sites must
state which regime it is in and must not "helpfully" unify them. If a third
regime ever seems necessary, that is a design discussion, not a constant.

## 7. Negative signal stays expensive

The bar for emitting `r⁻` is: textual or behavioral evidence, verified,
capped at 0.7, provenance-tracked. WPs that add negation sources (dismiss,
co-occurrence) must preserve all four properties. When in doubt, don't emit —
a false negative actively contradicts; a false positive merely dilutes.

## 8. Documentation moves with the code

- `docs/math-evolution.md` is the strategy ledger: when a WP ships something
  Part II describes, move the item to Part I (as-built, with any deviations
  added to the §5 ledger) in the same stack.
- `docs/whitepaper.md` describes current runtime behavior: update the affected
  section when behavior changes; add a "critical notes" entry for every
  hand-set constant introduced.
- This plan directory: set the WP's status in the README queue
  (`todo → in progress → shipped (PR #)`), and record any scope discovered but
  not done as a new WP at the appropriate stage — deferred work lives here as
  plan entries, not in chat.

## 9. Definition of done (every WP)

A WP is done when all of these hold:

1. Acceptance criteria in the WP's section are met, verified by running the
   stated commands (not by reading the diff).
2. `mise run check` green; matheval baselines updated if touched.
3. Docs updated per §8; stale text the WP was scoped to fix is actually fixed.
4. Fallback/degradation behavior exercised by at least one test (what happens
   when the corpus is tiny, the cache is cold, the input is degenerate).
5. The README queue row is updated with status and PR links.
