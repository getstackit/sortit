-- An overview memory (kind='overview') is the project's identity frame: one
-- paragraph stating what this project is. Unlike a concept it is a global
-- singleton with no subject tag — the analyzer reads it to ground every issue's
-- tagging in the project's identity. No new columns are needed (it reuses
-- title/body); the partial unique index enforces at most one active overview,
-- doubling as the supersede idempotency guard so concurrent writes cannot mint a
-- second active overview.
CREATE UNIQUE INDEX memories_overview_unique_idx
    ON memories (kind)
    WHERE kind = 'overview' AND status = 'active';
