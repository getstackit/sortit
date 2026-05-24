-- Reverse tags embedding_vector
DROP INDEX IF EXISTS tags_embedding_vector_cosine_hnsw_idx;
ALTER TABLE tags DROP COLUMN IF EXISTS embedding_vector;

-- Reverse events.issue_id FK and nullable change
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_issue_id_fkey;
UPDATE events SET issue_id = '' WHERE issue_id IS NULL;
ALTER TABLE events ALTER COLUMN issue_id SET DEFAULT '';
ALTER TABLE events ALTER COLUMN issue_id SET NOT NULL;

-- Reverse missing FK indexes
DROP INDEX IF EXISTS sessions_expires_at_unix_nano_idx;
DROP INDEX IF EXISTS issue_operation_participants_operation_id_idx;

-- Reverse issue_posts.kind index
DROP INDEX IF EXISTS issue_posts_kind_idx;
