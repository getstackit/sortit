DROP INDEX IF EXISTS tag_projections_embedding_vector_cosine_hnsw_idx;
DROP INDEX IF EXISTS tag_projections_canonical_name_idx;
DROP INDEX IF EXISTS tag_projections_status_name_idx;
DROP TABLE IF EXISTS tag_projections;

DROP INDEX IF EXISTS tag_events_tag_created_idx;
DROP INDEX IF EXISTS tag_events_source_idx;
DROP INDEX IF EXISTS tag_events_tag_sequence_idx;
DROP TABLE IF EXISTS tag_events;
