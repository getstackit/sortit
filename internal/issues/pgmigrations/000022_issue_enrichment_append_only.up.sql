CREATE TABLE issue_enrichment_events (
    id TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    target_sequence BIGINT NOT NULL,
    kind TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at_unix_nano BIGINT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'
);

CREATE TABLE issue_enrichment_projections (
    issue_id TEXT PRIMARY KEY REFERENCES issues(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'complete',
    error TEXT NOT NULL DEFAULT '',
    target_sequence BIGINT NOT NULL DEFAULT 1,
    attempt_count BIGINT NOT NULL DEFAULT 0,
    lease_expires_at_unix_nano BIGINT NOT NULL DEFAULT 0,
    latest_event_id TEXT NOT NULL DEFAULT '',
    updated_at_unix_nano BIGINT NOT NULL
);

CREATE INDEX issue_enrichment_events_issue_created_idx
ON issue_enrichment_events(issue_id, created_at_unix_nano DESC, id DESC);

CREATE INDEX issue_enrichment_events_target_idx
ON issue_enrichment_events(issue_id, target_sequence, created_at_unix_nano, id);

CREATE INDEX issue_enrichment_projections_status_idx
ON issue_enrichment_projections(status, target_sequence, issue_id);
