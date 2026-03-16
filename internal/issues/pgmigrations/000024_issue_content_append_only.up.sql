CREATE TABLE issue_content_facts (
    id TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    sequence BIGINT NOT NULL,
    kind TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at_unix_nano BIGINT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    inferred BOOLEAN NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX issue_content_facts_issue_sequence_idx
ON issue_content_facts(issue_id, sequence);

CREATE UNIQUE INDEX issue_content_facts_source_idx
ON issue_content_facts(source, source_id);

CREATE INDEX issue_content_facts_issue_created_idx
ON issue_content_facts(issue_id, created_at_unix_nano ASC, id ASC);

CREATE TABLE issue_content_projections (
    issue_id TEXT PRIMARY KEY REFERENCES issues(id) ON DELETE CASCADE,
    raw TEXT NOT NULL DEFAULT '',
    tags_json JSONB NOT NULL DEFAULT '[]',
    tag_scores_json JSONB NOT NULL DEFAULT '[]',
    embedding_vector vector,
    last_event_id TEXT NOT NULL DEFAULT '',
    event_count BIGINT NOT NULL DEFAULT 0,
    updated_at_unix_nano BIGINT NOT NULL
);

CREATE INDEX issue_content_projections_updated_idx
ON issue_content_projections(updated_at_unix_nano DESC, issue_id ASC);

CREATE INDEX issue_content_projections_embedding_vector_cosine_hnsw_idx
ON issue_content_projections
USING hnsw (((embedding_vector)::vector(1536)) vector_cosine_ops)
WHERE ((embedding_vector IS NOT NULL) AND (vector_dims(embedding_vector) = 1536));
