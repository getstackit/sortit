-- name: ListIssues :many
SELECT id, raw, tags_json, created_by, created_at_unix_nano, status, closed_at_unix_nano, closed_by, tag_scores_json, embedding_json
FROM issues
ORDER BY created_at_unix_nano DESC, id ASC;

-- name: GetIssue :one
SELECT id, raw, tags_json, created_by, created_at_unix_nano, status, closed_at_unix_nano, closed_by, tag_scores_json, embedding_json
FROM issues
WHERE id = ?;

-- name: ListIssuePosts :many
SELECT id, issue_id, raw, created_by, created_at_unix_nano, sequence, kind
FROM issue_posts
WHERE issue_id = ?
ORDER BY sequence ASC, created_at_unix_nano ASC, id ASC;

-- name: InsertIssue :exec
INSERT INTO issues (
    id,
    raw,
    tags_json,
    created_by,
    created_at_unix_nano,
    status,
    closed_at_unix_nano,
    closed_by,
    tag_scores_json,
    embedding_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertIssuePost :exec
INSERT INTO issue_posts (
    id,
    issue_id,
    raw,
    created_by,
    created_at_unix_nano,
    sequence,
    kind
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateIssueRefinement :exec
UPDATE issues
SET raw = ?,
    tags_json = ?,
    tag_scores_json = ?,
    embedding_json = ?
WHERE id = ?;

-- name: CloseIssue :exec
UPDATE issues
SET status = 'closed',
    closed_at_unix_nano = ?,
    closed_by = ?
WHERE id = ?;

-- name: ReopenIssue :exec
UPDATE issues
SET status = 'open',
    closed_at_unix_nano = 0,
    closed_by = ''
WHERE id = ?;

-- name: DeleteAllIssues :exec
DELETE FROM issues;

-- name: DeleteAllIssuePosts :exec
DELETE FROM issue_posts;

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
