-- name: ListEventsBefore :many
SELECT id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json
FROM events
WHERE created_at_unix_nano < $1
   OR (created_at_unix_nano = $1 AND id < $2)
ORDER BY created_at_unix_nano DESC, id DESC
LIMIT $3;
