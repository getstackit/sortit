# Data Model

Sortit stores durable state in PostgreSQL. The schema is migration-driven under `internal/issues/pgmigrations`, with `sqlc` query definitions in `internal/issues/sqlc`.

## Core Tables

`issues` is the compatibility current-state table for issue records. It stores:

- issue ID
- canonical raw text
- display tags
- tag score JSON
- embedding data/vector state
- creator and timestamps
- status and close fields
- assignee

`issue_posts` stores discussion entries. The first post is the initial report. Refinement and progress posts are stored with sequence numbers and kind metadata.

`issue_operations` and `issue_operation_participants` record grouped operations such as split and combine. Participants preserve issue roles and operation ordering.

`issue_links` stores explicit issue relationships:

- `parent_of`
- `child_of`
- `merged_into`
- `derived_from`
- `related_to`
- `duplicate_of`

`tags` stores the active tag catalog, descriptions, embeddings, specificity scores, and merge metadata added by later migrations.

Auth tables include `users`, `auth_accounts`, `sessions`, and `api_tokens`.

## Append-Only Facts And Projections

The current implementation keeps compatibility tables while adding append-only history and replayable projections.

Framework tables:

- `append_only_migration_checkpoints`
- `append_only_parity_runs`

Issue lifecycle:

- `issue_lifecycle_facts`
- `issue_lifecycle_projections`

Issue enrichment:

- `issue_enrichment_events`
- `issue_enrichment_projections`

Tags:

- `tag_events`
- `tag_projections`

Issue content:

- `issue_content_facts`
- `issue_content_projections`

Facts are immutable history. Projections are current-state read models rebuilt or updated from facts. Compatibility tables continue to support existing API behavior during migration and are still part of the active runtime.

## Persistence Rules

The append-only direction follows these invariants:

- Add fact tables and projections before replacing existing read paths.
- Preserve source identifiers, timestamps, actors, close semantics, links, and enrichment state.
- Make backfills idempotent and replayable.
- Keep projections queryable and cheap.
- Prefer parity checks and dual-write windows before any read cutover.
- Avoid treating worker lease tables as the only durable history.

## Enrichment State

Issue enrichment records include:

- canonical issue text
- display tags
- tag scores and verifier metadata
- embeddings
- target discussion sequence
- job attempts and status

The enrichment worker can claim pending jobs, update projections, and score affected tag specificity after enrichment.

## Tag State

Tags are normalized by name and can store:

- description
- embedding
- specificity
- specificity sub-scores
- computed timestamp
- status/canonical projection data

Suggested tags whose names start with `suggested-` are filtered out of the active runtime catalog by the catalog service. Tag merge and dismiss workflows are represented through tag persistence and tag-related API routes.

## Memory And Curation State

Memories are durable, permanent-by-default knowledge artifacts that share the tag
and embedding space with issues.

`memories` is the current-state table for memory records. It stores:

- memory ID, title, and body
- kind (`decision`, `lesson`, `constraint`, `pattern`, `reference`, `concept`)
- `subject_tag` — for `concept` memories only, the single tag the concept
  profiles. A concept is bound 1:1 to its subject tag; a partial unique index
  (`kind='concept' AND status='active'`) enforces at most one active concept per
  tag and serves as the synthesis idempotency guard. Empty for every other kind.
  Authoring a concept also **registers its subject tag in the catalog** (embedded
  from the concept body), so concepts grow the project's tagging vocabulary — see
  [Tag Taxonomy Health](./scoring-search-map.md#tag-taxonomy-health) and
  [onboarding.md](./onboarding.md).
- anchor tags (`anchor_tags_json`), anchor region, and tag scores
- embedding vector for semantic recall and map placement
- status and `superseded_by` (permanence is the default; supersede/archive are explicit)
- provenance: `source` and `source_issue_ids_json`
- confidence, creator, timestamps, and reinforcement signal
  (`last_reinforced_at_unix_nano`, `reinforcement_count`)

`memory_proposals` holds synthesized memory drafts awaiting human review, with
`rationale`, `confidence`, `status`, and `accepted_memory_id` once accepted.

`curation_proposals` holds propose-only librarian moves (combine, close-stale,
re-enrich, archive/supersede memory) that a human accepts or rejects; agents never
mutate the corpus directly.

## Search And Vector Data

Later migrations add vector columns and HNSW indexes for issue content and tag projections. The application still keeps compatibility JSON fields where needed, while vector-backed paths support semantic retrieval and map/search features.

## Schema Drift

The checked-in `internal/issues/sqlc/schema.sql` should match a freshly migrated PostgreSQL schema.

Use:

```bash
mise run check:schema-drift
```

Regenerate when intentional schema changes land:

```bash
mise run generate:schema
```
