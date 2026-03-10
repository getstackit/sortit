-- name: InsertIssuePost :exec
INSERT INTO issue_posts (
    id,
    issue_id,
    raw,
    created_by,
    created_at_unix_nano,
    sequence,
    kind
) VALUES ($1, $2, $3, $4, $5, $6, $7);
