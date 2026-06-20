-- name: ListIssuePostsForIssues :many
SELECT id, issue_id, raw, created_by, created_at_unix_nano, sequence, kind
FROM issue_posts
WHERE issue_id = ANY($1::text[])
ORDER BY issue_id ASC, sequence ASC, created_at_unix_nano ASC, id ASC;
