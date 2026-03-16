-- name: ListTags :many
SELECT name, description, created_at_unix_nano, embedding_json,
       specificity, specificity_llm, specificity_embedding, specificity_computed_at,
       embedding_vector
FROM tags
ORDER BY name ASC;
