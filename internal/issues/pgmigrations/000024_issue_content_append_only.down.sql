DROP INDEX IF EXISTS issue_content_projections_embedding_vector_cosine_hnsw_idx;
DROP INDEX IF EXISTS issue_content_projections_updated_idx;
DROP TABLE IF EXISTS issue_content_projections;

DROP INDEX IF EXISTS issue_content_facts_issue_created_idx;
DROP INDEX IF EXISTS issue_content_facts_source_idx;
DROP INDEX IF EXISTS issue_content_facts_issue_sequence_idx;
DROP TABLE IF EXISTS issue_content_facts;
