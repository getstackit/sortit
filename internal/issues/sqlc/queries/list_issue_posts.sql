-- name: ListIssuePosts :many
SELECT id, issue_id, raw, created_by, created_at_unix_nano, sequence, kind
FROM issue_posts
WHERE issue_id = $1
ORDER BY sequence ASC, created_at_unix_nano ASC, id ASC;
