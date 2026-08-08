-- SQLite baseline for the local-first edition. JSON documents are stored as
-- TEXT and validated by the application; embeddings remain JSON arrays until a
-- SQLite vector extension is introduced as an optional acceleration.

CREATE TABLE issues (
    id TEXT PRIMARY KEY,
    raw TEXT NOT NULL,
    tags_json TEXT NOT NULL DEFAULT '[]',
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    status TEXT NOT NULL,
    closed_at_unix_nano INTEGER NOT NULL DEFAULT 0,
    closed_by TEXT NOT NULL DEFAULT '',
    closed_reason TEXT NOT NULL DEFAULT '',
    closed_reason_note TEXT NOT NULL DEFAULT '',
    tag_scores_json TEXT NOT NULL DEFAULT '[]',
    embedding_json TEXT NOT NULL DEFAULT '[]',
    assigned_to TEXT NOT NULL DEFAULT '',
    enrichment_status TEXT NOT NULL DEFAULT 'complete',
    enrichment_error TEXT NOT NULL DEFAULT '',
    enrichment_target_sequence INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX issues_status_idx ON issues(status);
CREATE INDEX issues_assigned_to_idx ON issues(assigned_to);
CREATE INDEX issues_created_at_idx ON issues(created_at_unix_nano DESC, id);

CREATE TABLE issue_posts (
    id TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    raw TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    kind TEXT NOT NULL DEFAULT '',
    UNIQUE(issue_id, sequence)
);

CREATE INDEX issue_posts_issue_id_sequence_idx ON issue_posts(issue_id, sequence);

CREATE TABLE issue_operations (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    note TEXT NOT NULL DEFAULT ''
);

CREATE TABLE issue_operation_participants (
    operation_id TEXT NOT NULL REFERENCES issue_operations(id) ON DELETE CASCADE,
    issue_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    PRIMARY KEY (operation_id, issue_id, role)
);

CREATE INDEX issue_operation_participants_issue_id_idx ON issue_operation_participants(issue_id, sequence);

CREATE TABLE issue_links (
    id TEXT PRIMARY KEY,
    source_issue_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    target_issue_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    operation_id TEXT NOT NULL DEFAULT ''
);

CREATE INDEX issue_links_source_issue_id_idx ON issue_links(source_issue_id, created_at_unix_nano);
CREATE INDEX issue_links_target_issue_id_idx ON issue_links(target_issue_id, created_at_unix_nano);

CREATE TABLE tags (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    created_at_unix_nano INTEGER NOT NULL,
    embedding_json TEXT NOT NULL DEFAULT '[]',
    specificity REAL,
    specificity_llm REAL,
    specificity_embedding REAL,
    specificity_computed_at_unix_nano INTEGER
);

CREATE TABLE events (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    issue_id TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    participants_json TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX events_issue_id_created_at_idx ON events(issue_id, created_at_unix_nano);
CREATE INDEX events_kind_created_at_idx ON events(kind, created_at_unix_nano);

CREATE TABLE issue_enrichment_jobs (
    issue_id TEXT PRIMARY KEY REFERENCES issues(id) ON DELETE CASCADE,
    target_sequence INTEGER NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    available_at_unix_nano INTEGER NOT NULL,
    leased_until_unix_nano INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX issue_enrichment_jobs_available_idx ON issue_enrichment_jobs(available_at_unix_nano, leased_until_unix_nano);

CREATE TABLE map_projections (
    revision INTEGER PRIMARY KEY,
    payload BLOB NOT NULL,
    created_at_unix_nano INTEGER NOT NULL
);

CREATE TABLE custom_regions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    body_json TEXT NOT NULL DEFAULT '{}',
    created_by TEXT NOT NULL DEFAULT '',
    created_at_unix_nano INTEGER NOT NULL,
    updated_at_unix_nano INTEGER NOT NULL
);

CREATE TABLE memories (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'decision',
    anchor_tags_json TEXT NOT NULL DEFAULT '[]',
    anchor_region TEXT NOT NULL DEFAULT '',
    tag_scores_json TEXT NOT NULL DEFAULT '[]',
    embedding_json TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'active',
    superseded_by TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'manual',
    source_issue_ids_json TEXT NOT NULL DEFAULT '[]',
    created_by TEXT NOT NULL DEFAULT '',
    created_at_unix_nano INTEGER NOT NULL,
    updated_at_unix_nano INTEGER NOT NULL,
    subject_tag TEXT NOT NULL DEFAULT ''
);

CREATE INDEX memories_status_created_at_idx ON memories(status, created_at_unix_nano DESC, id);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    login TEXT NOT NULL,
    display_name TEXT NOT NULL,
    avatar_url TEXT NOT NULL,
    email TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    updated_at_unix_nano INTEGER NOT NULL
);

CREATE TABLE auth_accounts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    UNIQUE(provider, provider_user_id)
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at_unix_nano INTEGER NOT NULL,
    created_at_unix_nano INTEGER NOT NULL
);

CREATE TABLE api_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    revoked_at_unix_nano INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX auth_accounts_user_id_idx ON auth_accounts(user_id);
CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX api_tokens_user_id_idx ON api_tokens(user_id);
