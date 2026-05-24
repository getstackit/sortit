CREATE TABLE IF NOT EXISTS tag_merge_history (
    id              BIGSERIAL PRIMARY KEY,
    canonical_name  TEXT NOT NULL,
    alias_name      TEXT NOT NULL,
    merged_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    merged_by       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_tag_merge_history_canonical ON tag_merge_history (canonical_name);
CREATE INDEX IF NOT EXISTS idx_tag_merge_history_alias ON tag_merge_history (alias_name);

CREATE TABLE IF NOT EXISTS dismissed_tag_merges (
    id              BIGSERIAL PRIMARY KEY,
    canonical_name  TEXT NOT NULL,
    alias_name      TEXT NOT NULL,
    dismissed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (canonical_name, alias_name)
);
