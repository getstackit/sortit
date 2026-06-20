-- name: ListIssueLinksForIssues :many
SELECT id, source_issue_id, target_issue_id, type, created_by, created_at_unix_nano, note, operation_id
FROM issue_links
WHERE source_issue_id = ANY($1::text[]) OR target_issue_id = ANY($1::text[])
ORDER BY created_at_unix_nano DESC, id DESC;
