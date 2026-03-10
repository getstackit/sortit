CREATE SEQUENCE IF NOT EXISTS issue_seq START 1;
CREATE SEQUENCE IF NOT EXISTS issue_operation_seq START 1;

CREATE TABLE issues (
    id TEXT PRIMARY KEY,
    raw TEXT NOT NULL,
    tags_json JSONB NOT NULL DEFAULT '[]',
    created_by TEXT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    status TEXT NOT NULL,
    closed_at_unix_nano BIGINT NOT NULL,
    closed_by TEXT NOT NULL,
    tag_scores_json JSONB NOT NULL DEFAULT '[]',
    embedding_json JSONB NOT NULL DEFAULT '[]',
    assigned_to TEXT NOT NULL DEFAULT ''
);

CREATE INDEX issues_status_idx ON issues(status);
CREATE INDEX issues_assigned_to_idx ON issues(assigned_to);

CREATE TABLE issue_posts (
    id TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL,
    raw TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    sequence BIGINT NOT NULL,
    kind TEXT NOT NULL DEFAULT '',
    FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE,
    UNIQUE(issue_id, sequence)
);

CREATE INDEX issue_posts_issue_id_sequence_idx
ON issue_posts(issue_id, sequence);

CREATE TABLE issue_operations (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    note TEXT NOT NULL DEFAULT ''
);

CREATE TABLE issue_operation_participants (
    operation_id TEXT NOT NULL,
    issue_id TEXT NOT NULL,
    role TEXT NOT NULL,
    sequence BIGINT NOT NULL,
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
    created_at_unix_nano BIGINT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    operation_id TEXT NOT NULL DEFAULT '',
    FOREIGN KEY(source_issue_id) REFERENCES issues(id) ON DELETE CASCADE,
    FOREIGN KEY(target_issue_id) REFERENCES issues(id) ON DELETE CASCADE
);

CREATE INDEX issue_links_source_issue_id_idx
ON issue_links(source_issue_id, created_at_unix_nano);

CREATE INDEX issue_links_target_issue_id_idx
ON issue_links(target_issue_id, created_at_unix_nano);

CREATE TABLE tags (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    embedding_json JSONB NOT NULL DEFAULT '[]'
);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    login TEXT NOT NULL,
    display_name TEXT NOT NULL,
    avatar_url TEXT NOT NULL,
    email TEXT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    updated_at_unix_nano BIGINT NOT NULL
);

CREATE TABLE auth_accounts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(provider, provider_user_id)
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at_unix_nano BIGINT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE api_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    revoked_at_unix_nano BIGINT NOT NULL DEFAULT 0,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX auth_accounts_user_id_idx ON auth_accounts(user_id);
CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX api_tokens_user_id_idx ON api_tokens(user_id);
