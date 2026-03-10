-- name: ReopenIssue :exec
UPDATE issues
SET status = 'open',
    closed_at_unix_nano = 0,
    closed_by = ''
WHERE id = $1;
