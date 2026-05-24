ALTER TABLE issues
    ADD COLUMN IF NOT EXISTS enrichment_status TEXT NOT NULL DEFAULT 'complete',
    ADD COLUMN IF NOT EXISTS enrichment_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS enrichment_target_sequence BIGINT NOT NULL DEFAULT 1;

UPDATE issues
SET enrichment_status = 'complete',
    enrichment_error = '',
    enrichment_target_sequence = COALESCE((
        SELECT MAX(sequence)
        FROM issue_posts
        WHERE issue_posts.issue_id = issues.id
    ), 1);

CREATE TABLE IF NOT EXISTS issue_enrichment_jobs (
    issue_id TEXT PRIMARY KEY,
    target_sequence BIGINT NOT NULL,
    lease_expires_at_unix_nano BIGINT NOT NULL DEFAULT 0,
    attempt_count BIGINT NOT NULL DEFAULT 0,
    created_at_unix_nano BIGINT NOT NULL,
    updated_at_unix_nano BIGINT NOT NULL,
    FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS issue_enrichment_jobs_lease_idx
ON issue_enrichment_jobs(lease_expires_at_unix_nano, updated_at_unix_nano, issue_id);
