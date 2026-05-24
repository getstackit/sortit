-- name: CloseIssue :exec
UPDATE issues
SET status = 'closed',
    closed_at_unix_nano = $1,
    closed_by = $2,
    closed_reason = $3,
    closed_reason_note = $4
WHERE id = $5;
