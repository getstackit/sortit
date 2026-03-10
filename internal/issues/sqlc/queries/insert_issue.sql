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
    embedding_json,
    assigned_to
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);
