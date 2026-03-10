CREATE TABLE derived_corpus_projections (
    revision BIGINT PRIMARY KEY,
    payload_json JSONB NOT NULL,
    created_at_unix_nano BIGINT NOT NULL
);
