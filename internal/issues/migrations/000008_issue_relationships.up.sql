CREATE TABLE issue_operations (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    note TEXT NOT NULL DEFAULT ''
);

CREATE TABLE issue_operation_participants (
    operation_id TEXT NOT NULL,
    issue_id TEXT NOT NULL,
    role TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    PRIMARY KEY (operation_id, issue_id, role),
    FOREIGN KEY(operation_id) REFERENCES issue_operations(id) ON DELETE CASCADE,
    FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE
);

CREATE INDEX issue_operation_participants_issue_id_idx
ON issue_operation_participants(issue_id, sequence);

CREATE TABLE issue_links (
    id TEXT PRIMARY KEY,
    source_issue_id TEXT NOT NULL,
    target_issue_id TEXT NOT NULL,
    type TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    operation_id TEXT NOT NULL DEFAULT '',
    FOREIGN KEY(source_issue_id) REFERENCES issues(id) ON DELETE CASCADE,
    FOREIGN KEY(target_issue_id) REFERENCES issues(id) ON DELETE CASCADE,
    FOREIGN KEY(operation_id) REFERENCES issue_operations(id) ON DELETE SET DEFAULT
);

CREATE INDEX issue_links_source_issue_id_idx
ON issue_links(source_issue_id, created_at_unix_nano);

CREATE INDEX issue_links_target_issue_id_idx
ON issue_links(target_issue_id, created_at_unix_nano);

INSERT INTO metadata (key, value)
VALUES ('next_issue_operation_seq', '0')
ON CONFLICT(key) DO NOTHING;
