-- Standardize issue_snapshots embeddings on pgvector, matching every other
-- embedding column (issues, tags, issue_content_projections, memories).
-- issue_snapshots was the last table still storing embeddings as JSONB after the
-- embedding_json removal in migration 000018.
ALTER TABLE issue_snapshots
    ADD COLUMN IF NOT EXISTS embedding_vector vector;

-- Backfill the vector column from the JSONB array, skipping empty/non-array values
-- (mirrors the issues/tags backfill in 000007 / 000018). pgvector parses the JSON
-- array text "[0.1,0.2,...]" directly.
UPDATE issue_snapshots
SET embedding_vector = embedding_json::text::vector
WHERE embedding_vector IS NULL
  AND jsonb_typeof(embedding_json) = 'array'
  AND jsonb_array_length(embedding_json) > 0;

ALTER TABLE issue_snapshots DROP COLUMN embedding_json;
