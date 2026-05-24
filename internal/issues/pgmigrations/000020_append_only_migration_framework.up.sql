CREATE TABLE append_only_migration_checkpoints (
    name TEXT PRIMARY KEY,
    phase TEXT NOT NULL DEFAULT '',
    cursor_json JSONB NOT NULL DEFAULT '{}',
    summary_json JSONB NOT NULL DEFAULT '{}',
    updated_at_unix_nano BIGINT NOT NULL
);

CREATE TABLE append_only_parity_runs (
    id TEXT PRIMARY KEY,
    domain TEXT NOT NULL,
    status TEXT NOT NULL,
    details_json JSONB NOT NULL DEFAULT '{}',
    created_at_unix_nano BIGINT NOT NULL
);

CREATE INDEX append_only_parity_runs_domain_created_idx
ON append_only_parity_runs(domain, created_at_unix_nano DESC, id DESC);
