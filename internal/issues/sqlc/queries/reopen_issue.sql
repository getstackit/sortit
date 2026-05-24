-- name: ReopenIssue :exec
UPDATE issues
SET status = 'open',
    closed_at_unix_nano = 0,
    closed_by = '',
    closed_reason = '',
    closed_reason_note = ''
WHERE id = $1;
