-- name: InsertIssueLink :exec
INSERT INTO issue_links (
    id,
    source_issue_id,
    target_issue_id,
    type,
    created_by,
    created_at_unix_nano,
    note,
    operation_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
