CREATE TABLE issues (
    id TEXT PRIMARY KEY,
    raw TEXT NOT NULL,
    tags_json TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    status TEXT NOT NULL,
    closed_at_unix_nano INTEGER NOT NULL,
    closed_by TEXT NOT NULL,
    tag_scores_json TEXT NOT NULL,
    embedding_json TEXT NOT NULL,
    assigned_to TEXT NOT NULL DEFAULT ''
);

CREATE TABLE issue_posts (
    id TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL,
    raw TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    kind TEXT NOT NULL DEFAULT '',
    FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE,
    UNIQUE(issue_id, sequence)
);

CREATE INDEX issue_posts_issue_id_sequence_idx
ON issue_posts(issue_id, sequence);

CREATE TABLE metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE tags (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    embedding_json TEXT NOT NULL
);
