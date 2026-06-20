-- name: SearchIssuesByEmbedding :many
SELECT
  i.id,
  COALESCE(c.raw, i.raw) AS raw,
  COALESCE(c.tags_json, i.tags_json) AS tags_json,
  COALESCE(p.created_by, i.created_by) AS created_by,
  COALESCE(p.created_at_unix_nano, i.created_at_unix_nano) AS created_at_unix_nano,
  COALESCE(p.status, i.status) AS status,
  COALESCE(p.closed_at_unix_nano, 0) AS closed_at_unix_nano,
  COALESCE(p.closed_by, '') AS closed_by,
  COALESCE(p.closed_reason, '') AS closed_reason,
  COALESCE(p.closed_reason_note, '') AS closed_reason_note,
  COALESCE(c.tag_scores_json, i.tag_scores_json) AS tag_scores_json,
  COALESCE(c.embedding_vector::text, i.embedding_vector::text, '') AS embedding_text,
  COALESCE(p.assigned_to, i.assigned_to) AS assigned_to,
  (COALESCE(c.embedding_vector, i.embedding_vector) <=> sqlc.arg(query_vector)::vector) AS semantic_distance
FROM issues i
LEFT JOIN issue_content_projections c ON c.issue_id = i.id
LEFT JOIN issue_lifecycle_projections p ON p.issue_id = i.id
WHERE COALESCE(c.embedding_vector, i.embedding_vector) IS NOT NULL
  AND vector_dims(COALESCE(c.embedding_vector, i.embedding_vector)) = sqlc.arg(embedding_dims)
  AND (NOT sqlc.arg(filter_status)::bool OR COALESCE(p.status, i.status) = sqlc.arg(status)::text)
  AND (NOT sqlc.arg(filter_assigned_to)::bool OR LOWER(COALESCE(p.assigned_to, i.assigned_to)) = LOWER(sqlc.arg(assigned_to)::text))
  AND (NOT sqlc.arg(filter_exclude_id)::bool OR i.id <> sqlc.arg(exclude_id)::text)
  AND (
    NOT sqlc.arg(filter_tags)::bool
    OR EXISTS (
      SELECT 1
      FROM jsonb_array_elements_text(COALESCE(c.tags_json, i.tags_json)) AS tag
      WHERE LOWER(tag) = ANY(sqlc.arg(tags)::text[])
    )
    OR EXISTS (
      SELECT 1
      FROM jsonb_to_recordset(COALESCE(c.tag_scores_json, i.tag_scores_json)) AS score(tag text, relevance double precision)
      WHERE score.relevance >= 0.3
        AND LOWER(score.tag) = ANY(sqlc.arg(tags)::text[])
    )
  )
ORDER BY semantic_distance ASC, COALESCE(p.created_at_unix_nano, i.created_at_unix_nano) DESC, i.id ASC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);
