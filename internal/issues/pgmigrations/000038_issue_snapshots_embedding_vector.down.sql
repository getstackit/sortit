ALTER TABLE issue_snapshots
    ADD COLUMN IF NOT EXISTS embedding_json JSONB NOT NULL DEFAULT '[]';

-- Backfill embedding_json from embedding_vector (pgvector ::text is a JSON array).
UPDATE issue_snapshots
SET embedding_json = embedding_vector::text::jsonb
WHERE embedding_vector IS NOT NULL;

ALTER TABLE issue_snapshots DROP COLUMN embedding_vector;
