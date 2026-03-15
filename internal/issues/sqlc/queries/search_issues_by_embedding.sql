-- name: SearchIssuesByEmbedding :many
SELECT
  id,
  raw,
  tags_json,
  created_by,
  created_at_unix_nano,
  status,
  closed_at_unix_nano,
  closed_by,
  closed_reason,
  closed_reason_note,
  tag_scores_json,
  embedding_json,
  assigned_to,
  (embedding_vector <=> sqlc.arg(query_vector)::vector) AS semantic_distance
FROM issues
WHERE embedding_vector IS NOT NULL
  AND vector_dims(embedding_vector) = sqlc.arg(embedding_dims)
  AND (NOT sqlc.arg(filter_status)::bool OR status = sqlc.arg(status)::text)
  AND (NOT sqlc.arg(filter_assigned_to)::bool OR LOWER(assigned_to) = LOWER(sqlc.arg(assigned_to)::text))
  AND (NOT sqlc.arg(filter_exclude_id)::bool OR id <> sqlc.arg(exclude_id)::text)
  AND (
    NOT sqlc.arg(filter_tags)::bool
    OR EXISTS (
      SELECT 1
      FROM jsonb_array_elements_text(tags_json) AS tag
      WHERE LOWER(tag) = ANY(sqlc.arg(tags)::text[])
    )
    OR EXISTS (
      SELECT 1
      FROM jsonb_to_recordset(tag_scores_json) AS score(tag text, relevance double precision)
      WHERE score.relevance >= 0.3
        AND LOWER(score.tag) = ANY(sqlc.arg(tags)::text[])
    )
  )
ORDER BY semantic_distance ASC, created_at_unix_nano DESC, id ASC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);
