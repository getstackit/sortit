-- name: UpsertTag :exec
INSERT INTO tags (name, description, created_at_unix_nano, embedding_vector)
VALUES ($1, $2, $3, $4::vector)
ON CONFLICT(name) DO UPDATE SET
    description = CASE
        WHEN excluded.description <> '' THEN excluded.description
        ELSE tags.description
    END,
    embedding_vector = CASE
        WHEN excluded.embedding_vector IS NOT NULL THEN excluded.embedding_vector
        ELSE tags.embedding_vector
    END;
