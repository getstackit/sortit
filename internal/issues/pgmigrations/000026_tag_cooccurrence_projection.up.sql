-- tag_cooccurrence_projections stores corpus-wide pairwise tag co-occurrence
-- statistics. Single-row table (id = 'current') with the full pairwise matrix
-- in a JSONB body plus a revision stamp; consumers read the row and look up
-- the pair they need in memory.
--
-- The projection is wired up as a metric only. No mutation path or ranking
-- consumer reads it yet — see docs/math-evolution.md §4.1.
CREATE TABLE tag_cooccurrence_projections (
    id TEXT PRIMARY KEY,
    revision BIGINT NOT NULL DEFAULT 0,
    issue_count BIGINT NOT NULL DEFAULT 0,
    tag_count INT NOT NULL DEFAULT 0,
    body_json JSONB NOT NULL DEFAULT '{}',
    computed_at_unix_nano BIGINT NOT NULL DEFAULT 0
);
