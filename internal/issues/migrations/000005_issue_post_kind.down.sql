CREATE TABLE issue_posts_backup (
    id TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL,
    raw TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE,
    UNIQUE(issue_id, sequence)
);

INSERT INTO issue_posts_backup SELECT id, issue_id, raw, created_by, created_at_unix_nano, sequence FROM issue_posts;

DROP TABLE issue_posts;

ALTER TABLE issue_posts_backup RENAME TO issue_posts;

CREATE INDEX issue_posts_issue_id_sequence_idx ON issue_posts(issue_id, sequence);
