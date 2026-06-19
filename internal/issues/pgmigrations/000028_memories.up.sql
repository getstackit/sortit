-- memories stores permanent, tag/region-anchored knowledge artifacts. They
-- reuse the issue math primitives — a tag-relevance profile and a 1536-d
-- embedding indexed for cosine similarity — but carry their own permanent
-- lifecycle (reinforced, supersedable) rather than the open/closed issue
-- lifecycle. Memories are consumers of the issue-derived model and are kept in
-- a separate table so they never pollute the corpus statistics they derive from.
CREATE TABLE memories (
    id                           TEXT PRIMARY KEY,
    title                        TEXT NOT NULL DEFAULT '',
    body                         TEXT NOT NULL DEFAULT '',
    kind                         TEXT NOT NULL DEFAULT 'decision',
    anchor_tags_json             JSONB NOT NULL DEFAULT '[]',
    anchor_region                TEXT NOT NULL DEFAULT '',
    tag_scores_json              JSONB NOT NULL DEFAULT '[]',
    embedding_vector             vector,
    status                       TEXT NOT NULL DEFAULT 'active',
    superseded_by                TEXT NOT NULL DEFAULT '',
    source                       TEXT NOT NULL DEFAULT 'manual',
    source_issue_ids_json        JSONB NOT NULL DEFAULT '[]',
    confidence                   DOUBLE PRECISION NOT NULL DEFAULT 1,
    created_by                   TEXT NOT NULL DEFAULT '',
    created_at_unix_nano         BIGINT NOT NULL,
    updated_at_unix_nano         BIGINT NOT NULL,
    last_reinforced_at_unix_nano BIGINT NOT NULL DEFAULT 0,
    reinforcement_count          BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX memories_created_idx
    ON memories(created_at_unix_nano DESC, id);

CREATE INDEX memories_status_idx
    ON memories(status);

-- HNSW cosine index mirrors issue_content_projections so memory retrieval uses
-- the same approximate-nearest-neighbour path as issue search.
CREATE INDEX memories_embedding_vector_cosine_hnsw_idx
    ON memories
    USING hnsw (((embedding_vector)::vector(1536)) vector_cosine_ops)
    WHERE ((embedding_vector IS NOT NULL) AND (vector_dims(embedding_vector) = 1536));
