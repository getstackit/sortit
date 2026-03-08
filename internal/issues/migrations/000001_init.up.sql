CREATE TABLE IF NOT EXISTS issues (
    id TEXT PRIMARY KEY,
    raw TEXT NOT NULL,
    tags_json TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    tag_scores_json TEXT NOT NULL,
    embedding_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tags (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    embedding_json TEXT NOT NULL
);

INSERT INTO metadata (key, value)
VALUES ('next_issue_seq', '0')
ON CONFLICT(key) DO NOTHING;
