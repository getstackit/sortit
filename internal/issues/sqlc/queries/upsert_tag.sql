-- name: UpsertTag :exec
INSERT INTO tags (name, description, created_at_unix_nano, embedding_json)
VALUES ($1, $2, $3, $4)
ON CONFLICT(name) DO UPDATE SET
    description = CASE
        WHEN excluded.description <> '' THEN excluded.description
        ELSE tags.description
    END,
    embedding_json = CASE
        WHEN excluded.embedding_json <> '[]' THEN excluded.embedding_json
        ELSE tags.embedding_json
    END;
