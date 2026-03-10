-- name: CloseIssue :exec
UPDATE issues
SET status = 'closed',
    closed_at_unix_nano = $1,
    closed_by = $2
WHERE id = $3;
