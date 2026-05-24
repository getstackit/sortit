-- name: InsertIssueOperation :exec
INSERT INTO issue_operations (
    id,
    kind,
    created_by,
    created_at_unix_nano,
    note
) VALUES ($1, $2, $3, $4, $5);
