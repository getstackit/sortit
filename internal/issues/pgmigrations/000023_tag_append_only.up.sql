CREATE TABLE tag_events (
    id TEXT PRIMARY KEY,
    tag_name TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    kind TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at_unix_nano BIGINT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    inferred BOOLEAN NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX tag_events_tag_sequence_idx
ON tag_events(tag_name, sequence);

CREATE UNIQUE INDEX tag_events_source_idx
ON tag_events(source, source_id);

CREATE INDEX tag_events_tag_created_idx
ON tag_events(tag_name, created_at_unix_nano ASC, id ASC);

CREATE TABLE tag_projections (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    created_at_unix_nano BIGINT NOT NULL,
    embedding_vector vector,
    specificity REAL,
    specificity_llm REAL,
    specificity_embedding REAL,
    specificity_computed_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'active',
    canonical_name TEXT NOT NULL DEFAULT '',
    last_event_id TEXT NOT NULL DEFAULT '',
    event_count BIGINT NOT NULL DEFAULT 0,
    updated_at_unix_nano BIGINT NOT NULL
);

CREATE INDEX tag_projections_status_name_idx
ON tag_projections(status, name);

CREATE INDEX tag_projections_canonical_name_idx
ON tag_projections(canonical_name, name)
WHERE canonical_name <> '';

CREATE INDEX tag_projections_embedding_vector_cosine_hnsw_idx
ON tag_projections
USING hnsw (((embedding_vector)::vector(1536)) vector_cosine_ops)
WHERE ((embedding_vector IS NOT NULL) AND (vector_dims(embedding_vector) = 1536));
