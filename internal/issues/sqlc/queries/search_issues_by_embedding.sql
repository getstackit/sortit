-- name: SearchIssuesByEmbedding :many
SELECT
  i.id,
  i.raw,
  i.tags_json,
  COALESCE(p.created_by, i.created_by) AS created_by,
  COALESCE(p.created_at_unix_nano, i.created_at_unix_nano) AS created_at_unix_nano,
  COALESCE(p.status, i.status) AS status,
  COALESCE(p.closed_at_unix_nano, i.closed_at_unix_nano) AS closed_at_unix_nano,
  COALESCE(p.closed_by, i.closed_by) AS closed_by,
  COALESCE(p.closed_reason, i.closed_reason) AS closed_reason,
  COALESCE(p.closed_reason_note, i.closed_reason_note) AS closed_reason_note,
  i.tag_scores_json,
  COALESCE(i.embedding_vector::text, '') AS embedding_text,
  COALESCE(p.assigned_to, i.assigned_to) AS assigned_to,
  (i.embedding_vector <=> sqlc.arg(query_vector)::vector) AS semantic_distance
FROM issues i
LEFT JOIN issue_lifecycle_projections p ON p.issue_id = i.id
WHERE i.embedding_vector IS NOT NULL
  AND vector_dims(i.embedding_vector) = sqlc.arg(embedding_dims)
  AND (NOT sqlc.arg(filter_status)::bool OR COALESCE(p.status, i.status) = sqlc.arg(status)::text)
  AND (NOT sqlc.arg(filter_assigned_to)::bool OR LOWER(COALESCE(p.assigned_to, i.assigned_to)) = LOWER(sqlc.arg(assigned_to)::text))
  AND (NOT sqlc.arg(filter_exclude_id)::bool OR i.id <> sqlc.arg(exclude_id)::text)
  AND (
    NOT sqlc.arg(filter_tags)::bool
    OR EXISTS (
      SELECT 1
      FROM jsonb_array_elements_text(i.tags_json) AS tag
      WHERE LOWER(tag) = ANY(sqlc.arg(tags)::text[])
    )
    OR EXISTS (
      SELECT 1
      FROM jsonb_to_recordset(i.tag_scores_json) AS score(tag text, relevance double precision)
      WHERE score.relevance >= 0.3
        AND LOWER(score.tag) = ANY(sqlc.arg(tags)::text[])
    )
  )
ORDER BY semantic_distance ASC, COALESCE(p.created_at_unix_nano, i.created_at_unix_nano) DESC, i.id ASC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);
