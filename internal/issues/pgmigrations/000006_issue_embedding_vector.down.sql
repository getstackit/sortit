DROP INDEX IF EXISTS issues_embedding_vector_cosine_hnsw_idx;

ALTER TABLE issues
    DROP COLUMN IF EXISTS embedding_vector;

DROP EXTENSION IF EXISTS vector;
