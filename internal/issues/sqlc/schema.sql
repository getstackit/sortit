CREATE EXTENSION IF NOT EXISTS vector;

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
    closed_reason TEXT NOT NULL DEFAULT '',
    closed_reason_note TEXT NOT NULL DEFAULT '',
    tag_scores_json JSONB NOT NULL DEFAULT '[]',
    embedding_json JSONB NOT NULL DEFAULT '[]',
    embedding_vector vector,
    assigned_to TEXT NOT NULL DEFAULT '',
    enrichment_status TEXT NOT NULL DEFAULT 'complete',
    enrichment_error TEXT NOT NULL DEFAULT '',
    enrichment_target_sequence BIGINT NOT NULL DEFAULT 1
);

CREATE INDEX issues_status_idx ON issues(status);
CREATE INDEX issues_assigned_to_idx ON issues(assigned_to);
CREATE INDEX issues_embedding_vector_cosine_hnsw_idx
ON issues
USING hnsw ((embedding_vector::vector(1536)) vector_cosine_ops)
WHERE embedding_vector IS NOT NULL AND vector_dims(embedding_vector) = 1536;

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

CREATE INDEX issue_posts_kind_idx ON issue_posts(kind);

CREATE TABLE issue_snapshots (
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

CREATE INDEX issue_snapshots_issue_id_sequence_idx
ON issue_snapshots(issue_id, sequence);

CREATE TABLE issue_enrichment_jobs (
    issue_id TEXT PRIMARY KEY,
    target_sequence BIGINT NOT NULL,
    lease_expires_at_unix_nano BIGINT NOT NULL DEFAULT 0,
    attempt_count BIGINT NOT NULL DEFAULT 0,
    created_at_unix_nano BIGINT NOT NULL,
    updated_at_unix_nano BIGINT NOT NULL,
    FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE
);

CREATE INDEX issue_enrichment_jobs_lease_idx
ON issue_enrichment_jobs(lease_expires_at_unix_nano, updated_at_unix_nano, issue_id);

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

CREATE INDEX issue_operation_participants_operation_id_idx
ON issue_operation_participants(operation_id);

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

CREATE TABLE events (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    issue_id TEXT DEFAULT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    participants_json JSONB NOT NULL DEFAULT '[]',
    FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE SET NULL
);

CREATE INDEX events_created_at_idx ON events(created_at_unix_nano DESC, id DESC);
CREATE INDEX events_kind_idx ON events(kind);
CREATE INDEX events_issue_id_idx ON events(issue_id);

CREATE TABLE tags (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    created_at_unix_nano BIGINT NOT NULL,
    embedding_json JSONB NOT NULL DEFAULT '[]',
    specificity REAL,
    specificity_llm REAL,
    specificity_embedding REAL,
    specificity_computed_at TIMESTAMPTZ,
    embedding_vector vector
);

CREATE INDEX tags_embedding_vector_cosine_hnsw_idx
ON tags
USING hnsw ((embedding_vector::vector(1536)) vector_cosine_ops)
WHERE embedding_vector IS NOT NULL AND vector_dims(embedding_vector) = 1536;

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
CREATE INDEX sessions_expires_at_unix_nano_idx ON sessions(expires_at_unix_nano);
CREATE INDEX api_tokens_user_id_idx ON api_tokens(user_id);
