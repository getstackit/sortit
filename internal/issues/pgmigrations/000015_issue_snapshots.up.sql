CREATE TABLE IF NOT EXISTS issue_snapshots (
    issue_id TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    raw TEXT NOT NULL,
    tags_json JSONB NOT NULL DEFAULT '[]',
    tag_scores_json JSONB NOT NULL DEFAULT '[]',
    embedding_json JSONB NOT NULL DEFAULT '[]',
    created_at_unix_nano BIGINT NOT NULL,
    PRIMARY KEY (issue_id, sequence),
    FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS issue_snapshots_issue_id_sequence_idx
ON issue_snapshots(issue_id, sequence);
