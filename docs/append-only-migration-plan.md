# Append-Only Migration Plan

## Goal

Move Sortit toward append-only persistence without losing historical data or
breaking current reads and writes during the transition.

This document is the migration-safety plan for `01KKTCR4F3FZ0BE03G509K72HJ`.

## Non-goals

- Replacing all mutable tables in one cutover
- Rewriting or deleting existing source rows during the first migration phases
- Treating a new append-only model as correct before parity is proven
- Mixing schema cleanup work with canonical-data migration when cleanup is
  subordinate to the append-only design

## Migration invariants

Every append-only slice must satisfy these invariants:

- No dropped history: existing rows remain queryable until replacement history
  and projections are verified.
- No lossy transforms: backfills must preserve identifiers, timestamps, actor
  attribution, close semantics, link semantics, and enrichment state meaning.
- Additive first: introduce new fact tables and compatibility projections
  before repurposing old tables.
- Idempotent backfills: a backfill can be rerun safely without duplicate or
  diverging state.
- Parity before cutover: read paths do not switch until rebuilt projections
  match legacy behavior.
- Rollbackable cutover: legacy reads and writes remain available until the new
  path is proven stable.

## Current state inventory

The current schema already has some append-only-friendly tables, but the system
is not append-only overall.

### Mostly append-only today

- `issue_posts`
- `issue_operations`
- `issue_operation_participants`
- `issue_links`
- `events`
- `tag_merge_history`
- `dismissed_tag_merges`

These tables are closer to canonical facts, although some current workflows
still treat `issues` or `tags` as the real source of truth.

### Mutable source-of-truth or mutable state today

- `issues`
  - in-place updates for assign / close / reopen / refine / enrichment state
  - see `internal/issues/postgres_persistence.go`
- `issue_snapshots`
  - `ON CONFLICT DO UPDATE`
  - see `internal/issues/snapshots.go`
- `issue_enrichment_jobs`
  - upsert, claim-update, retry-update, complete-delete
  - see `internal/issues/enrichment_jobs_postgres.go`
- `tags`
  - upserted metadata, specificity updates, alias deletion during merges
  - see `internal/issues/postgres_persistence.go`
- `users`
  - profile updated in place
  - see `internal/auth/store.go`
- `sessions`
  - deleted on logout / invalidation
  - see `internal/auth/store.go`
- `api_tokens`
  - revoke and last-used mutate the live row
  - see `internal/auth/store.go`

### Projection or ephemeral state today

- `map_projections`
- `issue_enrichment_jobs`

These should remain explicit projections / worker-state surfaces even after the
append-only migration. The goal is not to eliminate projections; it is to stop
using them as the only durable truth.

## Target table roles

The target architecture should separate tables by role:

- Canonical append-only facts:
  immutable records for lifecycle transitions, snapshots, enrichment attempts,
  tag history, auth/session/token history
- Compatibility projections:
  current-state tables that preserve existing API and query behavior during
  migration
- Ephemeral worker state:
  lease-oriented tables or projections optimized for concurrency, rebuildable
  from canonical history

The exact table names can change, but the role boundaries should not.

## Recommended rollout order

Do the append-only migration in this order:

1. Cross-cutting migration framework
2. Issue lifecycle facts and `issues` compatibility projection
3. Snapshots and enrichment history
4. Tag history and tag projection
5. Auth / session / API-token history and projections

This order keeps the highest-value domain (`issues`) first while establishing
the migration machinery before touching every domain.

## Phase 0: Migration framework

Before introducing domain-specific fact tables, build the framework needed for
safe migration:

- a repeatable backfill runner with checkpointing
- idempotency rules for each domain
- parity check tooling between legacy tables and rebuilt projections
- cutover flags for read and write paths
- operational counters for drift, lag, and replay progress

This work is cross-cutting and should land before any irreversible decisions.

## Phase 1: Additive schema introduction

For each domain, add new append-only fact tables without removing current
tables.

Recommended families:

- issue lifecycle facts
  - create
  - assign
  - close
  - reopen
  - refine
- snapshot facts
- enrichment attempt / lease / completion history
- tag history facts
  - create
  - metadata change
  - merge
  - dismiss
  - alias relationship changes
- auth history facts
  - user profile observed / changed
  - session created / invalidated / expired
  - API token created / used / revoked

At this phase:

- no existing table should be dropped
- no current query should be forced onto the new facts yet
- compatibility projections should still be fed from the current write path

## Phase 2: Backfill legacy state into facts

Backfill is where data-loss risk is highest, so it must be explicit,
deterministic, and replayable.

### Issue lifecycle backfill

Backfill canonical lifecycle facts from:

- `issues`
- `issue_posts`
- `issue_operations`
- `issue_links`
- `events`

Rules:

- preserve existing issue IDs as aggregate identity
- preserve event / post ordering using stored timestamps and sequences
- reconstruct close / reopen / assign / refine semantics without discarding the
  current `issues` row
- treat the current `issues` row as a compatibility projection until the
  rebuilt projection matches it

### Snapshot and enrichment backfill

Backfill from:

- `issue_snapshots`
- `issue_enrichment_jobs`
- enrichment-related fields on `issues`

Rules:

- preserve the latest observed state even if the legacy queue is mutable
- encode claim / retry / complete semantics in a replayable history model
- do not delete current queue rows as part of backfill

### Tag backfill

Backfill from:

- `tags`
- `tag_merge_history`
- `dismissed_tag_merges`
- issue tag arrays / tag scores in `issues` and `issue_snapshots`

Rules:

- preserve canonical and alias identities
- do not treat deleted alias rows as permission to lose alias history
- retain merge chronology where known, and mark inferred history as inferred
  rather than pretending it was observed directly

### Auth / session / token backfill

Backfill from:

- `users`
- `auth_accounts`
- `sessions`
- `api_tokens`

Rules:

- preserve active sessions and revoked-token state
- treat missing logout history as incomplete legacy history, not as proof that
  no invalidation occurred
- preserve last-used and revoke timestamps as observed facts where available

## Phase 3: Rebuild candidate projections

Once facts exist and backfill has run, rebuild candidate projections from the
new fact tables.

Examples:

- rebuild `issues`-equivalent current state from lifecycle facts
- rebuild latest enrichment state from enrichment history
- rebuild active tag catalog from tag facts
- rebuild active sessions / tokens from auth history

These rebuilt projections should exist side-by-side with legacy tables first.
Do not switch reads yet.

## Phase 4: Parity checks

Cutover is blocked until parity checks pass.

Required parity checks:

- aggregate counts:
  - issue count
  - open / closed count
  - link count by type
  - snapshot count by issue
  - active session count
  - active / revoked token count
- field parity:
  - issue status
  - assignee
  - close fields
  - latest refinement raw
  - latest enrichment state
  - tag metadata
- sequence / ordering parity:
  - discussion sequence
  - snapshot sequence
  - operation participant ordering where relevant
- spot-check and sampling:
  - deterministic corpus fixtures
  - random production-like samples

Parity output should be machine-readable so CI or operational jobs can fail on
drift.

## Phase 5: Dual-write window

After parity exists, begin writing to the new append-only facts while
continuing to maintain the legacy-compatible projection path.

Rules for the dual-write window:

- append canonical facts first inside the transaction
- update the compatibility projection second inside the same transaction
- fail the write if canonical append fails
- do not remove legacy writes until drift has remained near zero over a stable
  observation window

This is the point where issue `01KKAXFE4A7P7VMQWB11YXG31A` should be folded into
the migration work: append and projection updates need atomic transaction
boundaries.

## Phase 6: Read cutover

Switch read paths only after all of the following are true:

- backfill is complete
- parity checks are green
- drift metrics remain acceptable during dual-write
- rollback path is exercised and documented
- compatibility projections rebuilt from facts behave like current APIs

Recommended order:

1. internal validation / shadow reads
2. non-critical reads
3. primary read APIs
4. admin / maintenance reads

Avoid switching every read surface at once.

## Phase 7: Legacy-path retirement

Only after sustained confidence should the legacy mutable-source path stop being
canonical.

Retirement criteria:

- append-only fact tables are fully populated
- rebuilt projections have matched production expectations over time
- rollback is no longer needed for ordinary incidents
- data recovery procedures are documented against the new facts

Even then:

- prefer demotion over deletion first
- keep old tables long enough for operational confidence and data recovery
- do schema cleanup only after canonical ownership has moved

This is why cleanup issues like nullable close fields should stay downstream of
the append-only lifecycle design rather than ahead of it.

## Rollback plan

Every cutover phase needs an explicit rollback:

- reads can point back to legacy tables
- writes can disable append-only dual-write if the new path is unhealthy
- backfills can be rerun from checkpoints
- projections can be rebuilt from facts again without destructive repair

Rollback should not require reconstructing data from logs outside the database.

## Domain-specific notes

### Issue lifecycle

- `issues` should become a compatibility projection, not disappear immediately
- `issue_posts`, `issue_operations`, `issue_links`, and `events` remain useful
  source material but are not sufficient by themselves to infer every current
  field without explicit lifecycle facts

### Snapshots and enrichment

- snapshot immutability is a prerequisite for trusting historical replay
- worker leases should remain operationally efficient, but lease state should
  not be the only durable history

### Tags

- tag alias deletion must stop being the only representation of merge outcome
- historical tag identities need explicit preservation, even if the current tag
  catalog presents a canonicalized view

### Auth

- session deletion and token mutation are convenient projections, not durable
  history
- security-sensitive lookup paths can still read current-state projections after
  append-only history becomes canonical

## Recommended next execution slices

1. Define the parity-check framework and migration checkpoints.
2. Implement the issue lifecycle fact schema and compatibility projection path.
3. Backfill lifecycle facts and build a projection comparator against `issues`.
4. Only then move on to snapshot / enrichment append-only work.
