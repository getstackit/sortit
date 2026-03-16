-- name: ListIssues :many
SELECT id, raw, tags_json, created_by, created_at_unix_nano, status, closed_at_unix_nano, closed_by, closed_reason, closed_reason_note, tag_scores_json, embedding_json, assigned_to
FROM issues
ORDER BY created_at_unix_nano DESC, id ASC;
