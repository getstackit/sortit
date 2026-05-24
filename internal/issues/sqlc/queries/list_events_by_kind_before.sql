-- name: ListEventsByKindBefore :many
SELECT id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json
FROM events
WHERE kind = $1
  AND (created_at_unix_nano < $2
       OR (created_at_unix_nano = $2 AND id < $3))
ORDER BY created_at_unix_nano DESC, id DESC
LIMIT $4;
