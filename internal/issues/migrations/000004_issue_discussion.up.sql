CREATE TABLE issue_posts (
    id TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL,
    raw TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE,
    UNIQUE(issue_id, sequence)
);

CREATE INDEX issue_posts_issue_id_sequence_idx
ON issue_posts(issue_id, sequence);

INSERT INTO issue_posts (id, issue_id, raw, created_by, created_at_unix_nano, sequence)
SELECT id || '-post-000001', id, raw, created_by, created_at_unix_nano, 1
FROM issues;
