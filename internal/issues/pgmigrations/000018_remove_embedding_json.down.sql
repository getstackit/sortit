ALTER TABLE issues ADD COLUMN embedding_json JSONB NOT NULL DEFAULT '[]';
ALTER TABLE tags ADD COLUMN embedding_json JSONB NOT NULL DEFAULT '[]';

-- Backfill embedding_json from embedding_vector
UPDATE issues
SET embedding_json = embedding_vector::text::jsonb
WHERE embedding_vector IS NOT NULL;

UPDATE tags
SET embedding_json = embedding_vector::text::jsonb
WHERE embedding_vector IS NOT NULL;
