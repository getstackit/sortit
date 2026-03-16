CREATE TABLE issue_lifecycle_facts (
    id TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    sequence BIGINT NOT NULL,
    kind TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at_unix_nano BIGINT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    inferred BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE issue_lifecycle_projections (
    issue_id TEXT PRIMARY KEY REFERENCES issues(id) ON DELETE CASCADE,
    created_by TEXT NOT NULL DEFAULT '',
    created_at_unix_nano BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    closed_at_unix_nano BIGINT NOT NULL DEFAULT 0,
    closed_by TEXT NOT NULL DEFAULT '',
    closed_reason TEXT NOT NULL DEFAULT '',
    closed_reason_note TEXT NOT NULL DEFAULT '',
    assigned_to TEXT NOT NULL DEFAULT '',
    last_fact_id TEXT NOT NULL DEFAULT '',
    fact_count BIGINT NOT NULL DEFAULT 0,
    updated_at_unix_nano BIGINT NOT NULL
);

CREATE INDEX issue_lifecycle_facts_issue_created_idx
ON issue_lifecycle_facts(issue_id, created_at_unix_nano, sequence, id);

CREATE UNIQUE INDEX issue_lifecycle_facts_source_idx
ON issue_lifecycle_facts(source, source_id);

CREATE INDEX issue_lifecycle_projections_status_idx
ON issue_lifecycle_projections(status, assigned_to, issue_id);
