# Stage 4 — Planning overlays

> Goal: the product payoff. Per-person, per-issue-set, and per-theme
> quantitative signals built on stable themes (Stage 2) and validated loadings.
> Queue: [README.md](./README.md).

Entry criteria: WP-204 shipped (themes with stable IDs behind an API) and its
soak criteria held. WP-302's person fixture strongly recommended before
WP-401's flip decision.
Exit criteria: the five overlay signals computable, tested, and exposed —
debug-tier first, promoted per-surface when a UI consumes them.

A shared principle for the whole stage: **overlays are read-model math, not
new state.** Every signal below is a deterministic function of the theme cache
(`W`, `H`, stable IDs), the decomposition cache (`f_i`), and existing
primitives (specificity, authority, lifecycle). The only new state in the
stage is WP-405's snapshots. Keep it that way — it is what makes these cheap
to build and safe to change.

---

## WP-401 — Person profile on validated loadings (shadow)

**Size:** M · **Depends on:** WP-204; use WP-302's person fixture for the
comparison if it exists.

### Context (verified)

The displayed person profile aggregates **raw analyzer relevance**:
`buildPersonTagProfile` → `issuemath.MeanTagProfile`, weighting
`ts.Relevance × specificityWeight` (`internal/people/person_profile.go`,
`internal/issuemath/profiles.go`). Ridge `f_i` enters person
*recommendations* (the blend in `person_detail.go`) but never the profile
itself. `work_correlations.go` similarly correlates raw profiles +
`MeanEmbedding`.

### Design

Two new profile constructions, shipped **parallel to** (not replacing) the
existing one:

```
tagProfile_f(person)   = mean over person's issues of f_i        ∈ ℝ^T
themeProfile(person)   = mean over person's issues of W_i        ∈ ℝ_+^K   (stable-ID keyed)
```

Decisions, made here:

- **Issue set:** same set the current profile uses (attributed/assigned
  issues), so the comparison isolates the representation change. Lifecycle
  weighting (recent issues weigh more) is a *later* refinement — do not bundle
  it into the representation A/B.
- **Negative loadings in `tagProfile_f`:** keep them (they mean "this
  person's work is geometrically *away* from tag k") but render only the
  positive side in any UI; the signed vector is for math consumers
  (correlations, fit).
- **Comparison before any flip:** a debug endpoint
  `GET /api/v1/debug/people/{id}/profiles` returning all three
  constructions side by side, plus — if WP-302's fixture landed — the
  recommendation-quality A/B using each profile as the seed. The existing
  profile is user-visible; it does not change until the comparison says the
  new one is better *and* a human has eyeballed the dev-corpus profiles.
- **Work correlations** get the same treatment later, driven by whichever
  representation wins; out of scope here beyond noting it.

### Steps

1. Profile builders reading the two caches (pure functions + service glue).
2. Debug endpoint with the three-way view.
3. Fixture comparison (WP-302 substrate): NDCG of `recommendOpenIssues`
   seeded by each profile construction; table into this document.
4. Flip decision recorded here with the data. If flipped: the profile
   surface switches in its own PR, old construction retained behind the
   debug endpoint for a soak window.

### Acceptance criteria

Three profile constructions computable and visible; a recorded, data-backed
flip/hold decision; no user-visible change without the comparison table.

---

## WP-402 — Issue-set theme coverage (iteration lens)

**Size:** S–M · **Depends on:** WP-204.

### Design

The general primitive, deliberately not iteration-specific:

```
coverage(issueSet) = normalize( Σ_{i ∈ set} W_i )   ∈ Δ^K  (stable-ID keyed)
```

- Service function + debug endpoint `POST /api/v1/debug/themes/coverage`
  taking an explicit issue-ID list and returning the theme distribution,
  per-theme contributors (top issues by W mass), and the share of the set's
  loading mass that fell outside all themes (the NMF reconstruction residual
  share — an honesty term: a set of issues the themes don't explain should
  say so, not silently normalize to 100%).
- "Iteration" is whatever issue set the caller assembles (a tag filter, a
  date window, an assignee's open issues). Sortit has no first-class
  iteration object today; this WP does not invent one. When one exists, it
  calls this primitive.
- Weighting choice: plain sum of W rows (mass-weighted — big-loading issues
  count more) vs mean of L1-normalized rows (issue-weighted). Default to
  issue-weighted (each issue votes once; planning is about counts of work,
  not loading magnitudes), expose the other behind a parameter, document the
  difference with an example.

### Acceptance criteria

Coverage endpoint with contributors and the unexplained-share term; unit
tests including the empty-set, single-issue, and no-theme-corpus cases;
worked example in the endpoint's doc comment.

---

## WP-403 — Gap analysis

**Size:** S · **Depends on:** WP-402.

### Design

Per stable theme `k`:

```
gap_score(k) = specificity(k) × staleness(k) × weight(k)

specificity(k) = Σ_t H_kt · tagSpecificity(t)          (loading-weighted mean; missing → 0.5)
staleness(k)   = 1 − coverage_recent(k)                 (coverage over issues touched in the window, default 90d)
weight(k)      = theme's share of corpus W mass         (a theme nobody ever worked on isn't a "gap")
```

Deviations from the sketch in math-evolution §8.4, with reasons:

- **Authority dropped from the product.** The whitepaper flags the authority
  slope as hand-set and asymmetric (§10.2 item 8, still open — WP-605);
  multiplying three heuristics is already at the edge of interpretability.
  Keep authority as a *displayed column* next to the gap score, not a factor
  inside it, until WP-605 gives it a footing.
- All three factors and the window are surfaced in the response so a human
  can see *why* something scored as a gap — a gap score nobody can decompose
  will not be trusted, and this overlay's entire value is trust.

Debug endpoint: `GET /api/v1/debug/themes/gaps?window=90d` — themes ranked by
gap score with the factor breakdown and each theme's most recent activity.

### Acceptance criteria

Ranked gaps with full factor decomposition; window parameterized; a
qualitative pass on the dev corpus recorded in the PR ("do the top gaps read
as real neglected areas?").

---

## WP-404 — Person-to-theme fit

**Size:** S · **Depends on:** WP-401 (theme profiles), WP-402.

### Design

```
fit(person, k) = cos( themeProfile(person), e_k )
```

where `e_k` is the stable-ID basis direction — equivalently, the person's
normalized W-mass share on theme k, smoothed toward the corpus prior for
people with few issues (additive smoothing, strength documented; a person
with 2 issues should not show fit 1.0 on one theme).

- Surfaces: per-person (`.../people/{id}/theme-fit`) and per-theme (top
  people for theme k — the routing view).
- Presentation rule, stated in the API docs and non-negotiable: fit is a
  *recommendation input*, never automatic assignment. The score answers
  "whose history matches this theme", which is not the same as "who should
  do this" (growth, load, and preference are human variables).
- Explicitly cheap: this is a lookup on WP-401 + WP-202 outputs. The WP is
  small precisely because the stages before it did the work — if it isn't
  small, something upstream leaked.

### Acceptance criteria

Both directions queryable; smoothing tested (few-issue people don't produce
degenerate fits); the recommendation-not-assignment rule in the docs.

---

## WP-405 — Theme drift over time + the snapshot decision

**Size:** M–L · **Depends on:** WP-402. The stage's only new-state WP.

### Design question first: where does history come from?

`drift(t) = 1 − cos(coverage(window t), coverage(window t−Δ))` needs theme
distributions for *past* windows. Two sources, decided here:

- **Option A — recompute from issue snapshots.** `issue_snapshots` (verified:
  `internal/issues/sqlc/schema.sql`) stores historical `tag_scores_json` and
  `embedding_vector` per issue revision. Reconstruct the corpus as of time t,
  run decomposition + theme projection against **today's** H (project old
  issues onto current themes — do NOT re-run NMF per window, which would
  create incomparable theme bases), and compute coverage. Zero new state;
  cost is one decomposition per requested window; correctness depends on
  snapshot completeness.
- **Option B — persist per-window aggregates going forward.** A small table
  of (window, stable theme ID, coverage share, issue count) written at
  refresh cadence. Cheap, simple, but blind before its birthday and coupled
  to refresh timing.

**Recommendation: A for the read path, B as a cache of A's outputs.**
Compute-on-demand from snapshots keeps one source of truth; persisting the
computed aggregates makes repeat queries cheap and survives snapshot
retention policies. Build A first; add B when the endpoint is slow enough to
care. Validate snapshot coverage on the dev corpus before committing to A
(if snapshots turn out sparse for old issues, B-going-forward is the honest
fallback and the endpoint says "history begins at …").

Also resolved here (flagged in WP-203): if Stage 4 surfaces are promoted to
user-visible, persist the **theme identity state** (stable ID → H row) so
process restarts don't reset identities. Same PR family as option B's table.

### Steps

1. Snapshot-coverage audit on the dev corpus (how far back can A see?).
2. `coverageAt(t, window)` via option A; drift endpoint
   `GET /api/v1/debug/themes/drift?delta=90d&points=4` returning the
   coverage series and pairwise drift values.
3. Aggregate-cache table (option B) only if measured latency demands it.
4. Theme-identity persistence if promotion is imminent.

### Acceptance criteria

Drift series over real history on the dev corpus; the A/B decision and
snapshot-coverage findings recorded in this document; no per-window NMF
re-runs (projection onto current H only, asserted by test).

### Risks

Projecting old issues onto current themes measures "how did focus move across
*today's* map of the work" — it cannot see themes that died so completely
that current NMF has no component for them. That is a real blind spot; the
gap analysis (WP-403) partially covers it (a dead theme with historical mass
still holds W mass corpus-wide). Document the limitation on the endpoint.

---

## Stage-4 exit checklist

- [ ] All five signals live at debug tier with tests and worked examples.
- [ ] WP-401 flip/hold decision recorded with comparison data.
- [ ] Snapshot decision (405) recorded; any new tables documented in
      data-model docs.
- [ ] math-evolution Part II Track B rewritten into Part I as-built prose.
- [ ] A demo pass on the dev corpus: profile → coverage → gaps → fit → drift,
      screenshotted or transcripted in the PR that closes the stage.
