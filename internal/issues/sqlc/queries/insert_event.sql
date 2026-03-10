-- name: InsertEvent :exec
INSERT INTO events (
    id,
    kind,
    issue_id,
    created_by,
    created_at_unix_nano,
    body,
    participants_json
) VALUES ($1, $2, $3, $4, $5, $6, $7);
