-- name: ListIssues :many
SELECT id, raw, tags_json, created_by, created_at_unix_nano, tag_scores_json, embedding_json
FROM issues
ORDER BY created_at_unix_nano DESC, id ASC;

-- name: GetIssue :one
SELECT id, raw, tags_json, created_by, created_at_unix_nano, tag_scores_json, embedding_json
FROM issues
WHERE id = ?;

-- name: InsertIssue :exec
INSERT INTO issues (
    id,
    raw,
    tags_json,
    created_by,
    created_at_unix_nano,
    tag_scores_json,
    embedding_json
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: DeleteAllIssues :exec
DELETE FROM issues;

-- name: ListTags :many
SELECT name, description, created_at_unix_nano, embedding_json
FROM tags
ORDER BY name ASC;

-- name: UpsertTag :exec
INSERT INTO tags (name, description, created_at_unix_nano, embedding_json)
VALUES (?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
    description = CASE
        WHEN excluded.description <> '' THEN excluded.description
        ELSE tags.description
    END,
    embedding_json = CASE
        WHEN excluded.embedding_json <> '[]' THEN excluded.embedding_json
        ELSE tags.embedding_json
    END;

-- name: GetMetadataValue :one
SELECT value
FROM metadata
WHERE key = ?;

-- name: UpdateMetadataValue :exec
UPDATE metadata
SET value = ?
WHERE key = ?;
