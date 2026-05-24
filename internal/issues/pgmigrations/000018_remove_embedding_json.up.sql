-- Backfill tags.embedding_vector from embedding_json where vector is NULL
UPDATE tags
SET embedding_vector = embedding_json::text::vector
WHERE embedding_vector IS NULL
  AND embedding_json IS NOT NULL
  AND embedding_json::text <> '[]'
  AND jsonb_array_length(embedding_json) > 0;

-- Safety backfill: issues.embedding_vector from embedding_json where vector is NULL
UPDATE issues
SET embedding_vector = embedding_json::text::vector
WHERE embedding_vector IS NULL
  AND embedding_json IS NOT NULL
  AND embedding_json::text <> '[]'
  AND jsonb_array_length(embedding_json) > 0;

ALTER TABLE issues DROP COLUMN embedding_json;
ALTER TABLE tags DROP COLUMN embedding_json;
