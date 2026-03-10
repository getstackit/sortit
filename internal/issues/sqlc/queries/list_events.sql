-- name: ListEvents :many
SELECT id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json
FROM events
ORDER BY created_at_unix_nano DESC, id DESC
LIMIT $1;
