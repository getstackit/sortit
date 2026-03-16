-- 01KKT2C3KRRAGY5YR83ZWTN2G2: Index on issue_posts.kind for filtering by post type
CREATE INDEX IF NOT EXISTS issue_posts_kind_idx ON issue_posts(kind);

-- 01KKT2BKTN6R8PVQ2MT7KNTN68: Missing indexes on FK columns
CREATE INDEX IF NOT EXISTS issue_operation_participants_operation_id_idx
ON issue_operation_participants(operation_id);

CREATE INDEX IF NOT EXISTS sessions_expires_at_unix_nano_idx
ON sessions(expires_at_unix_nano);

-- 01KKT2C0PKA11TY26G9BAQ8TWC: events.issue_id uses empty string default instead of NULL
-- Convert empty strings to NULL and make column nullable with FK constraint
ALTER TABLE events ALTER COLUMN issue_id DROP NOT NULL;
ALTER TABLE events ALTER COLUMN issue_id DROP DEFAULT;
ALTER TABLE events ALTER COLUMN issue_id SET DEFAULT NULL;
UPDATE events SET issue_id = NULL WHERE issue_id = '';
DO $$ BEGIN
    ALTER TABLE events ADD CONSTRAINT events_issue_id_fkey
        FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE SET NULL;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- 01KKT2BXDHG25WN2B6SJDV6KDZ: Add embedding_vector to tags for similarity search
ALTER TABLE tags ADD COLUMN IF NOT EXISTS embedding_vector vector;

CREATE INDEX IF NOT EXISTS tags_embedding_vector_cosine_hnsw_idx
ON tags
USING hnsw ((embedding_vector::vector(1536)) vector_cosine_ops)
WHERE embedding_vector IS NOT NULL AND vector_dims(embedding_vector) = 1536;
