CREATE TABLE events (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    issue_id TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    participants_json JSONB NOT NULL DEFAULT '[]'
);

CREATE INDEX events_created_at_idx ON events(created_at_unix_nano DESC, id DESC);
CREATE INDEX events_kind_idx ON events(kind);
CREATE INDEX events_issue_id_idx ON events(issue_id);
