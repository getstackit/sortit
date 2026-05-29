-- custom_regions stores user-defined region slices. A definition is a
-- small predicate over tag membership, status, author, and age. The
-- loader walks the issue corpus, applies the predicate to find members,
-- and reuses the existing region metric compute.
CREATE TABLE custom_regions (
    id              TEXT PRIMARY KEY,
    label           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    definition_json JSONB NOT NULL,
    created_by      TEXT NOT NULL DEFAULT '',
    created_at_unix_nano BIGINT NOT NULL,
    updated_at_unix_nano BIGINT NOT NULL
);

CREATE INDEX custom_regions_created_idx
    ON custom_regions(created_at_unix_nano DESC, id);
