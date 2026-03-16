-- name: ListIssues :many
SELECT
  i.id,
  COALESCE(c.raw, i.raw) AS raw,
  COALESCE(c.tags_json, i.tags_json) AS tags_json,
  COALESCE(p.created_by, i.created_by) AS created_by,
  COALESCE(p.created_at_unix_nano, i.created_at_unix_nano) AS created_at_unix_nano,
  COALESCE(p.status, i.status) AS status,
  COALESCE(p.closed_at_unix_nano, i.closed_at_unix_nano) AS closed_at_unix_nano,
  COALESCE(p.closed_by, i.closed_by) AS closed_by,
  COALESCE(p.closed_reason, i.closed_reason) AS closed_reason,
  COALESCE(p.closed_reason_note, i.closed_reason_note) AS closed_reason_note,
  COALESCE(c.tag_scores_json, i.tag_scores_json) AS tag_scores_json,
  COALESCE(c.embedding_vector::text, i.embedding_vector::text, '') AS embedding_text,
  COALESCE(p.assigned_to, i.assigned_to) AS assigned_to
FROM issues i
LEFT JOIN issue_content_projections c ON c.issue_id = i.id
LEFT JOIN issue_lifecycle_projections p ON p.issue_id = i.id
ORDER BY COALESCE(p.created_at_unix_nano, i.created_at_unix_nano) DESC, i.id ASC;
