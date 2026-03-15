-- name: GetIssue :one
SELECT id, raw, tags_json, created_by, created_at_unix_nano, status, closed_at_unix_nano, closed_by, tag_scores_json, embedding_json, assigned_to, enrichment_status, enrichment_error, enrichment_target_sequence
FROM issues
WHERE id = $1;
