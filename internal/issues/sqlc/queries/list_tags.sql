-- name: ListTags :many
SELECT name, description, created_at_unix_nano, embedding_json
FROM tags
ORDER BY name ASC;
