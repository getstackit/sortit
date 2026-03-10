CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE issues
    ADD COLUMN IF NOT EXISTS embedding_vector vector;

CREATE INDEX IF NOT EXISTS issues_embedding_vector_cosine_hnsw_idx
ON issues
USING hnsw ((embedding_vector::vector(1536)) vector_cosine_ops)
WHERE embedding_vector IS NOT NULL AND vector_dims(embedding_vector) = 1536;
