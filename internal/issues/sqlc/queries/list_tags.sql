-- name: ListTags :many
SELECT name, description, created_at_unix_nano,
       COALESCE(embedding_vector::text, '') AS embedding_text,
       specificity, specificity_llm, specificity_embedding, specificity_computed_at
FROM tag_projections
WHERE status = 'active'
ORDER BY name ASC;
