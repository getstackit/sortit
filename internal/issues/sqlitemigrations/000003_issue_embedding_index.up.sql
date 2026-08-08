-- The virtual table itself is created after the migration transaction. The
-- corresponding shadow table holds durable vectors for the Pure-Go sqlite-vec
-- index, which persists and invalidates its search structure automatically.
CREATE TABLE _vec_issue_embeddings (
    dataset_id TEXT NOT NULL,
    id TEXT NOT NULL,
    content TEXT,
    meta TEXT,
    embedding BLOB,
    PRIMARY KEY(dataset_id, id)
);

-- sqlite-vec stores the serialized search index and its short-lived build
-- lock separately from the source embeddings.
CREATE TABLE vector_storage (
    shadow_table_name TEXT NOT NULL,
    dataset_id TEXT NOT NULL DEFAULT '',
    "index" BLOB,
    PRIMARY KEY (shadow_table_name, dataset_id)
);

CREATE TABLE vector_storage_locks (
    shadow_table_name TEXT NOT NULL,
    dataset_id TEXT NOT NULL DEFAULT '',
    owner TEXT NOT NULL,
    locked_at INTEGER NOT NULL,
    PRIMARY KEY (shadow_table_name, dataset_id)
);
