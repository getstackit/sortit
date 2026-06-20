-- curation_proposals is the librarian review queue. A local, codebase-aware
-- agent drafts curation moves (combine duplicates, close stale issues,
-- re-enrich, archive/supersede memories); a human accepts or rejects each one.
-- Accepting dispatches to the underlying command handler. The per-kind target
-- reference is stored as a single JSONB payload so new kinds never require a
-- schema change.
CREATE TABLE curation_proposals (
    id                   TEXT PRIMARY KEY,
    kind                 TEXT NOT NULL DEFAULT 'combine_issues',
    payload_json         JSONB NOT NULL DEFAULT '{}',
    status               TEXT NOT NULL DEFAULT 'pending',
    rationale            TEXT NOT NULL DEFAULT '',
    confidence           DOUBLE PRECISION NOT NULL DEFAULT 0,
    source_refs_json     JSONB NOT NULL DEFAULT '[]',
    created_by           TEXT NOT NULL DEFAULT '',
    reviewed_by          TEXT NOT NULL DEFAULT '',
    result_ref           TEXT NOT NULL DEFAULT '',
    created_at_unix_nano BIGINT NOT NULL,
    updated_at_unix_nano BIGINT NOT NULL
);

CREATE INDEX curation_proposals_status_created_idx
    ON curation_proposals(status, created_at_unix_nano DESC, id);

CREATE INDEX curation_proposals_kind_idx
    ON curation_proposals(kind);
